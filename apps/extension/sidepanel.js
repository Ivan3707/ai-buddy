import { ApiError, chat, getMe, login } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import { clearToken, isUnauthorized, restoreSession, saveToken } from './popup-session.js';

const storage = chrome.storage.session;
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
let accessToken = '';

function showOnly(section) {
  sessionSection.hidden = section !== sessionSection;
  loginSection.hidden = section !== loginSection;
  assistantSection.hidden = section !== assistantSection;
  accountActions.hidden = section !== assistantSection;
}

function clearMessages() {
  messages.replaceChildren(Object.assign(document.createElement('p'), {
    className: 'hint',
    textContent: 'Спросите про отпуск, доступы или следующий шаг онбординга.',
  }));
}

function showLogin(message = '') {
  accessToken = '';
  passwordInput.value = '';
  clearMessages();
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
  await clearToken(storage);
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

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function appendMessage(node) {
  messages.querySelector('.hint')?.remove();
  messages.append(node);
  messages.scrollTop = messages.scrollHeight;
  return node;
}

function renderSources(sources) {
  const list = element('ul', 'sources');
  list.setAttribute('aria-label', 'Источники');
  for (const source of sources) {
    let url;
    try { url = new URL(source.url); } catch { continue; }
    if (url.protocol !== 'https:') continue;
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

loginForm.addEventListener('submit', async event => {
  event.preventDefault();
  const button = loginForm.querySelector('button');
  button.disabled = true;
  loginError.textContent = '';
  try {
    const result = await login(backend, $('#email').value.trim(), passwordInput.value);
    accessToken = result.access_token;
    await saveToken(storage, accessToken);
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

messageInput.addEventListener('keydown', event => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    chatForm.requestSubmit();
  }
});

chatForm.addEventListener('submit', async event => {
  event.preventDefault();
  const message = messageInput.value;
  if (!message.trim() || !accessToken) return;
  messageInput.value = '';
  appendMessage(element('div', 'msg msg-user', message));
  const bubble = appendMessage(element('div', 'msg msg-assistant'));
  const status = bubble.appendChild(element('p', 'status', 'Ищу в базе знаний…'));
  messageInput.disabled = sendButton.disabled = true;
  try {
    const result = await chat(backend, message, accessToken);
    status.remove();
    bubble.append(element('div', 'msg-text', result.answer));
    const sources = renderSources(result.sources);
    if (sources) bubble.append(sources);
  } catch (error) {
    status.remove();
    if (!(error instanceof ApiError && error.status === 401)) {
      bubble.append(element('div', 'msg-error', 'Backend временно недоступен. Повторите попытку позже.'));
    }
  } finally {
    messageInput.disabled = sendButton.disabled = false;
    messageInput.focus();
    messages.scrollTop = messages.scrollHeight;
  }
});

await verifyStoredSession();
