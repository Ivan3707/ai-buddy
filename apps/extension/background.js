// MV3 service worker.
// 1. Бади lives on every http(s) page. Clicking the toolbar icon opens/closes his chat on the current tab;
//    on pages where scripts cannot run (chrome://, Web Store) it opens the side panel instead.
// 2. Backend gateway for the on-page character "Бади" (buddy/widget.js + buddy/content.js): the content script runs in
//    the page, so it never sees the token and never calls the backend itself — it sends commands here
//    and receives ready-to-render view models.
// 3. Registers the character on all sites except the ones the user switched off
//    ("Убрать" in the side panel). On the AI-buddy web app it replaces the site's own widget (bridge in content.js).
import { getMe, login } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import { createGateway } from './lib/gateway.js';
import { readToken, saveToken } from './popup-session.js';

chrome.sidePanel
  .setPanelBehavior({openPanelOnActionClick: false})
  .catch(error => console.error('AI-buddy: cannot configure side panel', error));

chrome.action.onClicked.addListener(tab => {
  if (!tab.id) return;
  if (!/^https?:/.test(tab.url || '')) { void chrome.sidePanel.open({tabId: tab.id}); return; } // sync: keeps the user gesture
  void (async () => {
    try {
      await chrome.tabs.sendMessage(tab.id, {op: 'toggle'});
    } catch { // tab opened before install/reload, or a site switched off: inject now (activeTab allows it)
      try {
        await chrome.scripting.executeScript({target: {tabId: tab.id}, files: SCRIPTS});
        await chrome.tabs.sendMessage(tab.id, {op: 'toggle'});
      } catch (error) { console.error('AI-buddy: cannot show Бади on this tab', error); }
    }
  })();
});

const session = chrome.storage.session;
const OFF_KEY = 'buddyOffSites'; // sites where the user switched Бади off
const ALL_SITES = ['http://*/*', 'https://*/*'];
const SCRIPT_ID = 'ai-buddy-character';
const SCRIPTS = ['buddy/widget.js', 'buddy/content.js']; // the shared widget, then the extension's glue

// ---------- character registration ----------

async function offSites() {
  const stored = await chrome.storage.local.get(OFF_KEY);
  return Array.isArray(stored[OFF_KEY]) ? stored[OFF_KEY] : [];
}

async function syncRegistration() {
  const script = {
    id: SCRIPT_ID, js: SCRIPTS, matches: ALL_SITES, excludeMatches: await offSites(),
    runAt: 'document_idle', persistAcrossSessions: true,
  };
  const existing = await chrome.scripting.getRegisteredContentScripts({ids: [SCRIPT_ID]});
  if (existing.length) await chrome.scripting.updateContentScripts([script]);
  else await chrome.scripting.registerContentScripts([script]);
}

async function setSite(pattern, enabled, tabId) {
  const sites = (await offSites()).filter(site => site !== pattern);
  if (!enabled) sites.push(pattern);
  await chrome.storage.local.set({[OFF_KEY]: sites});
  await syncRegistration();
  if (tabId === undefined) return;
  if (enabled) {
    await chrome.scripting.executeScript({target: {tabId}, files: SCRIPTS});
  } else {
    await chrome.tabs.sendMessage(tabId, {op: 'remove'}).catch(() => {});
  }
}

chrome.runtime.onInstalled.addListener(() => { void syncRegistration(); });
chrome.runtime.onStartup.addListener(() => { void syncRegistration(); });

// ---------- backend gateway (lib/gateway.js, shared with the web app's Бади) ----------

const gateway = createGateway({
  baseUrl: backend,
  tokens: {get: () => readToken(session), set: token => saveToken(session, token)},
  store: { // conversation lives for the browser session; request statuses are remembered between sessions
    get: key => (key === 'conversationId' ? session : chrome.storage.local).get(key).then(stored => stored[key]),
    set: (key, value) => (key === 'conversationId' ? session : chrome.storage.local).set({[key]: value}),
    remove: key => (key === 'conversationId' ? session : chrome.storage.local).remove(key),
  },
  client: {getMe, login},
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
    case 'open-panel':
      // A click in the page is a user gesture that Chrome forwards with the message.
      await chrome.sidePanel.open({tabId: sender.tab.id});
      return {ok: true};
    case 'site-state':
      return {enabled: !(await offSites()).includes(message.pattern)};
    case 'set-site':
      await setSite(message.pattern, message.enabled, message.tabId);
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
