// MV3 service worker.
// 1. Бади has two states for the whole browser: SHOWN (in every window, at the same place) or HIDDEN (in no window).
//    The toolbar icon makes him shown (pressed again, it opens/closes his chat on this tab), the "×" on him hides him
//    everywhere. On pages where Chrome forbids scripts (chrome://, the new tab page, Web Store) he cannot be drawn:
//    the icon then only switches him to "shown" for the ordinary pages and says so with a badge.
// 2. Backend gateway for the on-page character "Бади" (buddy/widget.js + buddy/content.js): the content script runs in
//    the page, so it never sees the token and never calls the backend itself — it sends commands here
//    and receives ready-to-render view models.
// 3. Registers the character on all http(s) pages. On the AI-buddy web app it replaces the site's own widget
//    (bridge in content.js).
import { getMe, linkSession, login, logoutSession, refreshSession } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import { createAuth } from './lib/auth.js';
import { createGateway } from './lib/gateway.js';
import { decideSync } from './lib/sso.js';

// Chrome forbids extension scripts on chrome:// pages, the new tab page and the Web Store, so the character cannot appear
// there. The button still does its job (Бади becomes "shown" on every ordinary page) and a badge tells what happened.
async function showOnPlainPages(tabId) {
  await chrome.storage.local.set({buddyVisible: true});
  try {
    await chrome.action.setBadgeBackgroundColor({color: '#a7cb77', tabId});
    await chrome.action.setBadgeText({text: '✓', tabId});
    await chrome.action.setTitle({title: 'Бади показан на обычных страницах. Здесь Chrome не разрешает расширениям рисовать.', tabId});
    setTimeout(() => {
      void chrome.action.setBadgeText({text: '', tabId}).catch(() => {});
      void chrome.action.setTitle({title: 'Открыть AI-buddy', tabId}).catch(() => {});
    }, 4000);
  } catch { /* the tab was closed */ }
}

const VISIBLE_KEY = 'buddyVisible'; // false = hidden in every window; anything else = shown in every window
chrome.action.onClicked.addListener(tab => {
  if (!tab.id) return;
  if (!/^https?:/.test(tab.url || '')) { void showOnPlainPages(tab.id); return; }
  void (async () => {
    const wasShown = (await chrome.storage.local.get(VISIBLE_KEY))[VISIBLE_KEY] !== false;
    await chrome.storage.local.set({[VISIBLE_KEY]: true}); // every tab shows him through storage.onChanged
    const toggle = () => chrome.tabs.sendMessage(tab.id, {op: 'toggle', wasShown});
    try {
      await toggle();
    } catch { // tab opened before install/reload: inject now (activeTab allows it)
      try {
        await chrome.scripting.executeScript({target: {tabId: tab.id}, files: SCRIPTS});
        await toggle();
      } catch (error) { console.error('AI-buddy: cannot show Бади on this tab', error); }
    }
  })();
});

const ALL_SITES = ['http://*/*', 'https://*/*'];
const SCRIPT_ID = 'ai-buddy-character';
const SCRIPTS = ['buddy/bridge.js', 'buddy/widget.js', 'buddy/content.js']; // guarded link to the worker, the shared widget, the glue

// ---------- character registration ----------

async function syncRegistration() {
  const script = {
    id: SCRIPT_ID, js: SCRIPTS, matches: ALL_SITES,
    runAt: 'document_idle', persistAcrossSessions: true,
  };
  const existing = await chrome.scripting.getRegisteredContentScripts({ids: [SCRIPT_ID]});
  if (existing.length) await chrome.scripting.updateContentScripts([script]);
  else await chrome.scripting.registerContentScripts([script]);
}

chrome.runtime.onInstalled.addListener(() => { void syncRegistration(); });
chrome.runtime.onStartup.addListener(() => { void syncRegistration(); });

// ---------- session (lib/auth.js, shared with the web app) ----------
// The session lives in chrome.storage.local: it survives closing tabs and restarting the browser. Only this worker
// holds the tokens; pages and content scripts never see them.

const local = {
  get: key => chrome.storage.local.get(key).then(stored => stored[key]),
  set: (key, value) => chrome.storage.local.set({[key]: value}),
  remove: key => chrome.storage.local.remove(key),
};
const auth = createAuth({baseUrl: backend, storage: local, client: {login, refreshSession, linkSession, logoutSession}});

// ---------- single sign-on with the AI-buddy web app ----------
// The site and the extension keep separate sessions of the same user (lib/auth.js). On a handshake the site sends
// {sub, loginAt, signedOutAt}; lib/sso.js decides who adopts whose session. Only a tab of the site's own origin may do this.
const WEB_ORIGINS = ['http://127.0.0.1:5173', 'http://localhost:5173'];
const fromWebApp = sender => {
  try { return Boolean(sender?.tab) && WEB_ORIGINS.includes(new URL(sender.url || '').origin); } catch { return false; }
};
const fromOwnPage = sender => !sender?.tab && String(sender?.url || '').startsWith(chrome.runtime.getURL(''));
const num = value => (Number.isFinite(value) ? value : 0);

// Tell the open site tabs that the extension signed in/out, so they run a handshake.
async function notifySiteTabs() {
  const tabs = await chrome.tabs.query({url: WEB_ORIGINS.map(origin => `${origin}/*`)}).catch(() => []);
  for (const tab of tabs) chrome.tabs.sendMessage(tab.id, {op: 'sso-changed'}).catch(() => {});
}

async function adoptPair(pair, loginAt) {
  const before = (await auth.info()).sub;
  await auth.adopt(pair, {loginAt});
  if ((await auth.info()).sub !== before) await chrome.storage.session.remove('conversationId'); // another user: new dialog
}

async function handshake(site) {
  const ext = await auth.info();
  const action = decideSync({sub: String(site?.sub || ''), loginAt: num(site?.loginAt), signedOutAt: num(site?.signedOutAt)}, ext);
  if (action === 'pull') {
    const pair = await auth.link();
    return pair ? {action: 'pull', pair, loginAt: ext.loginAt} : {action: 'none'};
  }
  if (action === 'ext-logout') { // the site signed out after our sign-in, while the extension was not listening
    await auth.logout();
    await chrome.storage.session.remove('conversationId');
    return {action: 'none'};
  }
  return {action};
}

// ---------- backend gateway (lib/gateway.js, shared with the web app's Бади) ----------

const gateway = createGateway({
  baseUrl: backend,
  auth,
  hooks: {onLogin: () => void notifySiteTabs(), onLogout: () => void notifySiteTabs()},
  store: { // conversation lives for the browser session; request statuses are remembered between sessions
    get: key => (key === 'conversationId' ? chrome.storage.session : chrome.storage.local).get(key).then(stored => stored[key]),
    set: (key, value) => (key === 'conversationId' ? chrome.storage.session : chrome.storage.local).set({[key]: value}),
    remove: key => (key === 'conversationId' ? chrome.storage.session : chrome.storage.local).remove(key),
  },
  client: {getMe},
});

// Long-lived port per page: streams answers to the character as view models.
chrome.runtime.onConnect.addListener(port => {
  if (port.name !== 'buddy') return;
  let abort = null;
  port.onDisconnect.addListener(() => abort?.());
  port.onMessage.addListener(message => {
    if (message.op === 'abort') { abort?.(); return; }
    if (message.op !== 'chat' && message.op !== 'brief') return;
    abort?.();
    const requestId = message.requestId;
    abort = gateway.stream(message, payload => { try { port.postMessage({requestId, ...payload}); } catch { /* page gone */ } });
  });
});

async function handle(message, sender) {
  switch (message.op) {
    case 'sso-handshake':
      if (!fromWebApp(sender)) return {error: 'forbidden'};
      return handshake(message.state);
    case 'sso-adopt': { // the site opened a session for us (POST /api/auth/link)
      const pair = message.pair;
      if (!fromWebApp(sender) || !pair || typeof pair.access_token !== 'string' || typeof pair.refresh_token !== 'string') return {error: 'forbidden'};
      await adoptPair(pair, num(message.loginAt) || Date.now());
      return {ok: true};
    }
    case 'sso-logout': // the user signed out on the site
      if (!fromWebApp(sender)) return {error: 'forbidden'};
      await auth.logout();
      await chrome.storage.session.remove('conversationId');
      return {ok: true};
    default:
      return gateway.call(message);
  }
}

// Only our own pages and content scripts can reach this listener (no externally_connectable).
chrome.runtime.onMessage.addListener((message, sender, reply) => {
  handle(message, sender).then(reply, error => reply({error: String(error?.message || error)}));
  return true; // async reply
});
