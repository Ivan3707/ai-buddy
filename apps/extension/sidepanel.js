import { chat, getMe } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import {
  actionTitle, describeActionError, describeActionStatus, describeParams, describeResult,
} from './lib/actions.js';
import { cancelAction, confirmAction, sendFeedback, streamBrief, streamChat } from './lib/api.js';
import { briefStorageKey, shouldRequestBrief, todayKey } from './lib/brief.js';
import {
  applyStreamEvent, createAssistantState, failureCode, finalizeStream, statusLabel,
} from './lib/stream-events.js';
import { collectPageContext, contextLabel, describeActiveTab } from './lib/page-context.js';
import { isUnauthorized } from './popup-session.js';

const storage = chrome.storage.session;
const conversationKey = 'conversationId';
const $ = selector => document.querySelector(selector);
const sessionSection = $('#session-section');
const sessionMessage = $('#session-message');
const retrySession = $('#retry-session');
const loginSection = $('#login-section');
const assistantSection = $('#assistant-section');
const accountActions = $('#account-actions');
const loginForm = $('#login');
const loginError = $('#login-error');
const accountEmail = $('#account-email');
const passwordInput = $('#password');
const chatForm = $('#chat');
const messageInput = $('#message');
const sendButton = $('#send');
const messages = $('#messages');
const modeHint = $('#mode-hint');
const usePageToggle = $('#use-page');
const pageStatus = $('#page-status');
const pagePreferenceKey = 'usePageContext';

let accessToken = '';
let busy = false;
let currentRequest = null; // AbortController of the running stream
let dialogGeneration = 0; // bumps on "new dialog"/logout so late results are dropped
// Learned once per panel lifetime: the current backend has no /api/chat/stream yet.
let streamingSupported = true;

// ---------- session ----------
// The session (tokens, refresh, single sign-on with the site) belongs to the background worker; the panel only
// asks it for a fresh access token before it talks to the backend.

async function freshToken() {
  const reply = await chrome.runtime.sendMessage({op: 'token'}).catch(() => null);
  accessToken = typeof reply?.token === 'string' ? reply.token : '';
  return accessToken;
}

function showOnly(section) {
  sessionSection.hidden = section !== sessionSection;
  loginSection.hidden = section !== loginSection;
  assistantSection.hidden = section !== assistantSection;
  accountActions.hidden = section !== assistantSection;
}

function showLogin(message = '') {
  accessToken = '';
  passwordInput.value = '';
  resetDialogView();
  loginError.textContent = message;
  showOnly(loginSection);
}

function showAssistant(user) {
  accountEmail.textContent = user.email;
  loginError.textContent = '';
  showOnly(assistantSection);
  messageInput.focus();
  void refreshPageStatus();
  void maybeShowBrief(user.id);
}

function showSessionCheck(message = 'Проверяем сессию…', canRetry = false) {
  sessionMessage.textContent = message;
  retrySession.hidden = !canRetry;
  showOnly(sessionSection);
}

async function logout(message = '') {
  accessToken = '';
  abortCurrent();
  await Promise.all([chrome.runtime.sendMessage({op: 'logout'}).catch(() => null), storage.remove(conversationKey)]);
  showLogin(message);
}

async function verifyStoredSession() {
  showSessionCheck();
  // A hanging request must not leave the panel on "Проверяем сессию…" forever.
  const withTimeout = request => Promise.race([
    request, new Promise((_, reject) => setTimeout(() => reject(new Error('timeout')), 8000)),
  ]);
  const token = await freshToken();
  if (!token) { showLogin(''); return; }
  try {
    showAssistant(await withTimeout(getMe(backend, token)));
  } catch (error) {
    if (isUnauthorized(error)) { showLogin('Сессия истекла. Войдите снова.'); return; }
    showSessionCheck('Backend временно недоступен. Вход сохранён, попробуйте ещё раз.', true);
  }
}

// ---------- rendering ----------

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function nearBottom() {
  return messages.scrollHeight - messages.scrollTop - messages.clientHeight < 48;
}

function appendMessage(node) {
  messages.querySelector('.hint')?.remove();
  messages.append(node);
  messages.scrollTop = messages.scrollHeight;
  return node;
}

function resetDialogView() {
  messages.replaceChildren(element('p', 'hint', 'Спросите про отпуск, доступы или следующий шаг онбординга.'));
}

function renderSources(sources) {
  const list = element('ul', 'sources');
  list.setAttribute('aria-label', 'Источники');
  for (const source of sources) {
    let url;
    try { url = new URL(source.url); } catch { continue; }
    if (url.protocol !== 'https:') continue; // never render javascript:/data: links
    const link = element('a', '', /демо/i.test(source.title) ? source.title : `${source.title} (демо)`);
    link.href = url.href;
    link.target = '_blank';
    link.rel = 'noreferrer';
    if (source.snippet) link.title = source.snippet;
    const item = element('li');
    item.append(link);
    list.append(item);
  }
  return list.childElementCount ? list : null;
}

// Card for a PendingAction: nothing is executed until the user clicks "Подтвердить".
function renderActionCard(action) {
  const title = actionTitle(action.type);
  const card = element('section', 'action-card');
  card.setAttribute('aria-label', title);
  card.append(element('h2', 'action-title', title));
  if (action.summary) card.append(element('p', 'action-summary', action.summary));
  const rows = describeParams(action.type, action.params);
  if (rows.length) {
    const list = element('dl', 'action-params');
    for (const {label, value} of rows) list.append(element('dt', '', label), element('dd', '', value));
    card.append(list);
  }
  const result = element('p', 'action-result');
  result.setAttribute('role', 'status');
  result.hidden = true;
  const showResult = (message, tone) => {
    result.textContent = message;
    result.dataset.tone = tone;
    result.hidden = !message;
  };

  if (action.status !== 'pending') {
    showResult(describeActionStatus(action), action.status === 'failed' ? 'error' : 'muted');
    card.append(result);
    return card;
  }

  const buttons = element('div', 'action-buttons');
  const confirm = element('button', '', 'Подтвердить');
  const cancel = element('button', 'secondary', 'Отменить');
  confirm.type = cancel.type = 'button';
  buttons.append(confirm, cancel);
  card.append(buttons, result);

  async function run(operation) {
    confirm.disabled = cancel.disabled = true;
    card.setAttribute('aria-busy', 'true');
    showResult(operation === 'confirm' ? 'Отправляю заявку…' : 'Отменяю…', 'muted');
    const options = {baseUrl: backend, token: await freshToken()};
    try {
      if (operation === 'confirm') {
        const outcome = describeResult(await confirmAction(options, action.id));
        showResult(outcome.text, outcome.tone);
      } else {
        await cancelAction(options, action.id);
        showResult('Отменено. Заявка не отправлена.', 'muted');
      }
      buttons.remove();
    } catch (error) {
      if (isUnauthorized(error)) return;
      const {text, retryable} = describeActionError(error?.status, operation);
      showResult(text, 'error');
      if (retryable) confirm.disabled = cancel.disabled = false;
      else buttons.remove();
    } finally {
      card.removeAttribute('aria-busy');
    }
  }

  confirm.addEventListener('click', () => void run('confirm'));
  cancel.addEventListener('click', () => void run('cancel'));
  return card;
}

// 👍/👎 under an answer; shown only when the backend sent meta.message_id.
function renderFeedback(messageId) {
  const row = element('div', 'feedback');
  row.setAttribute('role', 'group');
  row.setAttribute('aria-label', 'Оцените ответ');
  const note = element('span', 'feedback-note');
  note.setAttribute('aria-live', 'polite');
  const buttons = [['up', '👍', 'Полезный ответ'], ['down', '👎', 'Бесполезный ответ']].map(([rating, icon, label]) => {
    const button = element('button', 'feedback-button', icon);
    button.type = 'button';
    button.setAttribute('aria-label', label);
    button.title = label;
    button.setAttribute('aria-pressed', 'false');
    button.addEventListener('click', async () => {
      for (const other of buttons) other.disabled = true;
      button.setAttribute('aria-pressed', 'true');
      try {
        await sendFeedback({baseUrl: backend, token: await freshToken()}, messageId, rating);
        note.textContent = 'Спасибо за оценку';
      } catch (error) {
        if (isUnauthorized(error)) return;
        button.setAttribute('aria-pressed', 'false');
        for (const other of buttons) other.disabled = false;
        note.textContent = 'Не удалось сохранить оценку';
      }
    });
    return button;
  });
  row.append(...buttons, note);
  return row;
}

// One assistant bubble. render(state) is idempotent and cheap enough per delta.
function createAssistantView({onRetry} = {}) {
  const root = element('div', 'msg msg-assistant');
  const status = root.appendChild(element('p', 'status'));
  status.setAttribute('role', 'status');
  const text = root.appendChild(element('div', 'msg-text'));
  let sourcesNode = null;
  let renderedSources = null;
  let errorNode = null;
  let actionNode = null;
  let renderedActionId = null;
  let feedbackNode = null;

  function render(state) {
    const stick = nearBottom();
    const label = statusLabel(state);
    status.hidden = !label;
    status.textContent = label;
    text.textContent = state.text || (state.finished && !state.error && !state.action ? 'Ответ пуст.' : '');
    root.setAttribute('aria-busy', String(!state.finished));

    if (state.sources !== renderedSources) {
      renderedSources = state.sources;
      sourcesNode?.remove();
      sourcesNode = renderSources(state.sources);
      if (sourcesNode) text.after(sourcesNode);
    }

    if ((state.action?.id ?? null) !== renderedActionId) {
      renderedActionId = state.action?.id ?? null;
      actionNode?.remove();
      actionNode = state.action ? renderActionCard(state.action) : null;
      if (actionNode) (sourcesNode || text).after(actionNode);
    }

    const wantsFeedback = state.finished && !state.error && Boolean(state.messageId);
    if (!wantsFeedback && feedbackNode) {
      feedbackNode.remove();
      feedbackNode = null;
    } else if (wantsFeedback && !feedbackNode) {
      feedbackNode = root.appendChild(renderFeedback(state.messageId));
    }

    errorNode?.remove();
    errorNode = null;
    if (state.error) {
      errorNode = element('div', 'msg-error');
      errorNode.setAttribute('role', 'alert');
      errorNode.append(element('span', '', state.error.message));
      if (onRetry) {
        const retry = element('button', '', 'Повторить');
        retry.type = 'button';
        retry.disabled = busy;
        retry.addEventListener('click', () => onRetry());
        errorNode.append(retry);
      }
      root.append(errorNode);
    }
    if (stick) messages.scrollTop = messages.scrollHeight;
  }

  return {root, render};
}

function setBusy(value) {
  busy = value;
  messageInput.disabled = value;
  sendButton.disabled = value;
  for (const retry of messages.querySelectorAll('.msg-error button')) retry.disabled = value;
  if (!value) messageInput.focus();
}

function abortCurrent() {
  dialogGeneration += 1;
  currentRequest?.abort();
  currentRequest = null;
}

async function readConversationId() {
  const stored = await storage.get(conversationKey);
  return typeof stored[conversationKey] === 'string' ? stored[conversationKey] : '';
}

// ---------- page context ----------

const PAGE_STATUS_TEXT = {
  restricted: 'Служебная страница — её контекст не передаётся.',
  'no-access': 'Нет доступа к этой вкладке. Нажмите на иконку AI-buddy на панели браузера, чтобы разрешить.',
  'no-tab': '',
};

async function refreshPageStatus() {
  if (!usePageToggle.checked) {
    pageStatus.textContent = 'Контекст страницы не передаётся.';
    return;
  }
  const preview = await describeActiveTab(chrome);
  pageStatus.textContent = preview.state === 'ready' ? `Страница: ${preview.title}` : PAGE_STATUS_TEXT[preview.state];
}

async function loadPagePreference() {
  try {
    const stored = await chrome.storage.local.get(pagePreferenceKey);
    usePageToggle.checked = stored[pagePreferenceKey] !== false; // default: on
  } catch {
    usePageToggle.checked = true;
  }
}

async function pageContextForRequest(userNode) {
  if (!usePageToggle.checked) return null;
  const {context} = await collectPageContext(chrome);
  if (context && userNode && !userNode.querySelector('.msg-context')) {
    const chip = element('span', 'msg-context', `Страница: ${contextLabel(context)}`);
    if (context.selection) chip.textContent += ' · с выделенным текстом';
    userNode.append(chip);
  }
  return context;
}

// ---------- chat ----------

async function ask(message, view, userNode) {
  if (busy || !accessToken) return;
  setBusy(true);
  await freshToken();
  const generation = dialogGeneration;
  const controller = new AbortController();
  currentRequest = controller;
  let state = createAssistantState();
  view.render(state);

  try {
    const conversationId = await readConversationId();
    // Legacy fallback cannot carry page_context, so the page is not even read then.
    const pageContext = streamingSupported ? await pageContextForRequest(userNode) : null;
    const request = {
      message,
      ...(conversationId ? {conversation_id: conversationId} : {}),
      ...(pageContext ? {page_context: pageContext} : {}),
    };
    const result = await streamChat({
      baseUrl: backend,
      token: accessToken,
      request,
      signal: controller.signal,
      preferLegacy: !streamingSupported,
      legacyChat: text => chat(backend, text, accessToken),
      onEvent: event => {
        if (generation !== dialogGeneration) return;
        state = applyStreamEvent(state, event);
        view.render(state);
      },
    });
    if (result.transport === 'legacy') {
      // Legacy /api/chat does not accept page_context: do not claim it was sent.
      userNode?.querySelector('.msg-context')?.remove();
      streamingSupported = false;
      modeHint.hidden = false;
    }
    if (result.conversationReset && generation === dialogGeneration && !state.conversationId) {
      await storage.remove(conversationKey);
    }
    state = finalizeStream(state);
  } catch (error) {
    if (isUnauthorized(error)) return; // the unauthorized event already logs out
    state = controller.signal.aborted ? finalizeStream(state, 'interrupted') : finalizeStream(state, failureCode(error));
  } finally {
    if (currentRequest === controller) currentRequest = null;
    if (generation === dialogGeneration) setBusy(false);
    else busy = false;
  }

  if (generation !== dialogGeneration) return;
  if (state.conversationId) await storage.set({[conversationKey]: state.conversationId});
  view.render(state);
}

// Proactive greeting with the next onboarding step, once per user per day.
async function maybeShowBrief(userId) {
  if (busy || !(await freshToken())) return;
  const today = todayKey();
  const key = briefStorageKey(userId);
  let lastBriefDate;
  try { lastBriefDate = (await chrome.storage.local.get(key))[key]; } catch { /* treat as never shown */ }
  const hasConversation = Boolean(await readConversationId()) || Boolean(messages.querySelector('.msg'));
  if (!shouldRequestBrief({userId, lastBriefDate, today, hasConversation})) return;

  setBusy(true);
  const generation = dialogGeneration;
  const controller = new AbortController();
  currentRequest = controller;
  const view = createAssistantView();
  let state = createAssistantState();
  let shown = false;
  try {
    const result = await streamBrief({
      baseUrl: backend,
      token: accessToken,
      signal: controller.signal,
      onEvent: event => {
        if (generation !== dialogGeneration) return;
        if (!shown) {
          shown = true;
          view.root.classList.add('msg-brief');
          appendMessage(view.root);
        }
        state = applyStreamEvent(state, event);
        view.render(state);
      },
    });
    if (result.transport === 'unsupported') return; // backend without the brief yet
    state = finalizeStream(state);
  } catch (error) {
    state = finalizeStream(state, controller.signal.aborted ? 'interrupted' : failureCode(error));
  } finally {
    if (currentRequest === controller) currentRequest = null;
    if (generation === dialogGeneration) setBusy(false);
    else busy = false;
  }
  if (generation !== dialogGeneration) return;
  if (shown) view.render(state);
  if (!state.error) {
    await chrome.storage.local.set({[key]: today}).catch(() => {});
    if (state.conversationId) await storage.set({[conversationKey]: state.conversationId});
  }
}

function startQuestion(message) {
  const userNode = appendMessage(element('div', 'msg msg-user', message));
  const view = createAssistantView({onRetry: () => void ask(message, view, userNode)});
  appendMessage(view.root);
  void ask(message, view, userNode);
}

// ---------- events ----------

loginForm.addEventListener('submit', async event => {
  event.preventDefault();
  const button = loginForm.querySelector('button');
  button.disabled = true;
  loginError.textContent = '';
  try {
    const result = await chrome.runtime.sendMessage({op: 'login', email: $('#email').value.trim(), password: passwordInput.value});
    if (!result?.ok) throw Object.assign(new Error(result?.error || 'login failed'), {status: result?.error?.startsWith('Неверный') ? 401 : 503});
    await storage.remove(conversationKey);
    showAssistant(await getMe(backend, await freshToken()));
  } catch (error) {
    loginError.textContent = isUnauthorized(error)
      ? 'Неверный email или пароль.'
      : 'Backend временно недоступен. Попробуйте войти ещё раз.';
  } finally {
    passwordInput.value = '';
    button.disabled = false;
  }
});

$('#logout').addEventListener('click', () => { void logout(); });
retrySession.addEventListener('click', () => { void verifyStoredSession(); });
globalThis.addEventListener('ai-buddy:unauthorized', () => {
  if (accessToken) void logout('Сессия истекла. Войдите снова.');
});

$('#new-chat').addEventListener('click', async () => {
  abortCurrent();
  setBusy(false);
  await storage.remove(conversationKey);
  resetDialogView();
  messageInput.value = '';
  messageInput.focus();
});

messageInput.addEventListener('keydown', event => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    chatForm.requestSubmit();
  }
});

chatForm.addEventListener('submit', event => {
  event.preventDefault();
  const message = messageInput.value.trim();
  if (!message || busy || !accessToken) return;
  messageInput.value = '';
  startQuestion(message);
});

usePageToggle.addEventListener('change', () => {
  void chrome.storage.local.set({[pagePreferenceKey]: usePageToggle.checked}).catch(() => {});
  void refreshPageStatus();
});
// The tabs API events need no permission; without access the url/title are just absent.
chrome.tabs.onActivated.addListener(() => { void refreshPageStatus(); });
chrome.tabs.onUpdated.addListener((_tabId, change) => {
  if (change.status === 'complete' || change.title || change.url) void refreshPageStatus();
});
globalThis.addEventListener('focus', () => { void refreshPageStatus(); });

try {
  await loadPagePreference();
  await verifyStoredSession();
} catch (error) {
  showSessionCheck(`Не удалось открыть панель: ${error?.message || error}. Нажмите «Проверить снова».`, true);
}
