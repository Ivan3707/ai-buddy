import { chat, getMe, login } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import {
  actionTitle, describeActionError, describeActionStatus, describeParams, describeResult,
} from './lib/actions.js';
import { cancelAction, confirmAction, streamChat } from './lib/api.js';
import {
  applyStreamEvent, createAssistantState, failureCode, finalizeStream, statusLabel,
} from './lib/stream-events.js';
import { clearToken, isUnauthorized, restoreSession, saveToken } from './popup-session.js';

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

let accessToken = '';
let busy = false;
let currentRequest = null; // AbortController of the running stream
let dialogGeneration = 0; // bumps on "new dialog"/logout so late results are dropped
// Learned once per panel lifetime: the current backend has no /api/chat/stream yet.
let streamingSupported = true;

// ---------- session ----------

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

function showAssistant(email) {
  accountEmail.textContent = email;
  loginError.textContent = '';
  showOnly(assistantSection);
  messageInput.focus();
}

function showSessionCheck(message = 'Проверяем сессию…', canRetry = false) {
  sessionMessage.textContent = message;
  retrySession.hidden = !canRetry;
  showOnly(sessionSection);
}

async function logout(message = '') {
  accessToken = '';
  abortCurrent();
  await Promise.all([clearToken(storage), storage.remove(conversationKey)]);
  showLogin(message);
}

async function verifyStoredSession() {
  showSessionCheck();
  const restored = await restoreSession(storage, token => getMe(backend, token));
  if (restored.status === 'authenticated') {
    accessToken = restored.token;
    showAssistant(restored.user.email);
    return;
  }
  if (restored.status === 'unavailable') {
    accessToken = restored.token;
    showSessionCheck('Backend временно недоступен. Токен сохранён, попробуйте ещё раз.', true);
    return;
  }
  showLogin(restored.status === 'unauthorized' ? 'Сессия истекла. Войдите снова.' : '');
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
    const link = element('a', '', `${source.title} (демо)`);
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
    const options = {baseUrl: backend, token: accessToken};
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

// ---------- chat ----------

async function readConversationId() {
  const stored = await storage.get(conversationKey);
  return typeof stored[conversationKey] === 'string' ? stored[conversationKey] : '';
}

async function ask(message, view) {
  if (busy || !accessToken) return;
  setBusy(true);
  const generation = dialogGeneration;
  const controller = new AbortController();
  currentRequest = controller;
  let state = createAssistantState();
  view.render(state);

  try {
    const conversationId = await readConversationId();
    const request = {message, ...(conversationId ? {conversation_id: conversationId} : {})};
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
    if (result.transport === 'legacy' && streamingSupported) {
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

function startQuestion(message) {
  appendMessage(element('div', 'msg msg-user', message));
  const view = createAssistantView({onRetry: () => void ask(message, view)});
  appendMessage(view.root);
  void ask(message, view);
}

// ---------- events ----------

loginForm.addEventListener('submit', async event => {
  event.preventDefault();
  const button = loginForm.querySelector('button');
  button.disabled = true;
  loginError.textContent = '';
  try {
    const result = await login(backend, $('#email').value.trim(), passwordInput.value);
    accessToken = result.access_token;
    await Promise.all([saveToken(storage, accessToken), storage.remove(conversationKey)]);
    showAssistant(result.user.email);
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

await verifyStoredSession();
