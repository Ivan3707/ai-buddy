import { ApiError, chat, getMe, login } from './client.js';
import { clearToken, isUnauthorized, restoreSession, saveToken } from './popup-session.js';

const backend = 'http://127.0.0.1:8080';
const storage = chrome.storage.session;
const sessionSection = document.querySelector('#session-section');
const sessionMessage = document.querySelector('#session-message');
const retrySession = document.querySelector('#retry-session');
const loginSection = document.querySelector('#login-section');
const assistantSection = document.querySelector('#assistant-section');
const loginForm = document.querySelector('#login');
const chatForm = document.querySelector('#chat');
const output = document.querySelector('#reply');
const loginError = document.querySelector('#login-error');
const accountEmail = document.querySelector('#account-email');
const passwordInput = document.querySelector('#password');
let accessToken = '';

function showOnly(section) {
  sessionSection.hidden = section !== sessionSection;
  loginSection.hidden = section !== loginSection;
  assistantSection.hidden = section !== assistantSection;
}

function showLogin(message = '') {
  accessToken = '';
  passwordInput.value = '';
  output.textContent = '';
  loginError.textContent = message;
  showOnly(loginSection);
}

function showAssistant(email) {
  accountEmail.textContent = email;
  loginError.textContent = '';
  showOnly(assistantSection);
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

loginForm.addEventListener('submit', async event => {
  event.preventDefault();
  const emailInput = document.querySelector('#email');
  const button = loginForm.querySelector('button');
  button.disabled = true;
  loginError.textContent = '';
  try {
    const result = await login(backend, emailInput.value.trim(), passwordInput.value);
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

document.querySelector('#logout').addEventListener('click', () => {
  void logout();
});

retrySession.addEventListener('click', () => {
  void verifyStoredSession();
});

globalThis.addEventListener('ai-buddy:unauthorized', () => {
  if (accessToken) void logout('Сессия истекла. Войдите снова.');
});

chatForm.addEventListener('submit', async event => {
  event.preventDefault();
  const message = document.querySelector('#message').value;
  if (!message.trim() || !accessToken) return;
  const button = chatForm.querySelector('button');
  button.disabled = true;
  output.textContent = 'Ищем…';
  try {
    const result = await chat(backend, message, accessToken);
    output.textContent = result.answer;
    for (const source of result.sources) {
      const link = document.createElement('a');
      const url = new URL(source.url);
      if (url.protocol !== 'https:') continue;
      link.href = url.href;
      link.textContent = `${source.title} (демо)`;
      link.target = '_blank';
      link.rel = 'noreferrer';
      output.append(document.createElement('br'), link);
    }
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401)) {
      output.textContent = 'Backend временно недоступен. Повторите попытку позже.';
    }
  } finally {
    button.disabled = false;
  }
});

await verifyStoredSession();
