// MV3 service worker.
// 1. Бади lives on every http(s) page. Clicking the toolbar icon opens/closes his chat on the current tab;
//    on pages where scripts cannot run (chrome://, Web Store) it opens the side panel instead.
// 2. Backend gateway for the on-page character "Бади" (buddy/content.js): the content script runs in
//    the page, so it never sees the token and never calls the backend itself — it sends commands here
//    and receives ready-to-render view models.
// 3. Registers the character on all sites except the ones the user switched off (side panel "Убрать")
//    The AI-buddy web app has its own Бади widget, so content.js skips it (except the /demo/ pages).
import { getMe, login } from './client.js';
import { BACKEND_ORIGIN as backend } from './config.js';
import { describeActionError, describeResult } from './lib/actions.js';
import { cancelAction, confirmAction, sendFeedback, streamBrief, streamChat } from './lib/api.js';
import { displayName, pageHint, requestStatusChanges, toView } from './lib/buddy.js';
import { applyStreamEvent, createAssistantState, failureCode, finalizeStream } from './lib/stream-events.js';
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
        await chrome.scripting.executeScript({target: {tabId: tab.id}, files: ['buddy/content.js']});
        await chrome.tabs.sendMessage(tab.id, {op: 'toggle'});
      } catch (error) { console.error('AI-buddy: cannot show Бади on this tab', error); }
    }
  })();
});

const session = chrome.storage.session;
const OFF_KEY = 'buddyOffSites'; // sites where the user switched Бади off
const ALL_SITES = ['http://*/*', 'https://*/*'];
const SCRIPT_ID = 'ai-buddy-character';

// ---------- character registration ----------

async function offSites() {
  const stored = await chrome.storage.local.get(OFF_KEY);
  return Array.isArray(stored[OFF_KEY]) ? stored[OFF_KEY] : [];
}

async function syncRegistration() {
  const script = {
    id: SCRIPT_ID, js: ['buddy/content.js'], matches: ALL_SITES, excludeMatches: await offSites(), // web app pages are skipped by content.js itself
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
    await chrome.scripting.executeScript({target: {tabId}, files: ['buddy/content.js']});
  } else {
    await chrome.tabs.sendMessage(tabId, {op: 'remove'}).catch(() => {});
  }
}

chrome.runtime.onInstalled.addListener(() => { void syncRegistration(); });
chrome.runtime.onStartup.addListener(() => { void syncRegistration(); });

// ---------- backend gateway ----------

async function conversationId() {
  const stored = await session.get('conversationId');
  return typeof stored.conversationId === 'string' ? stored.conversationId : '';
}

// Long-lived port per page: streams answers to the character as view models.
chrome.runtime.onConnect.addListener(port => {
  if (port.name !== 'buddy') return;
  let controller = null;
  port.onDisconnect.addListener(() => controller?.abort());
  port.onMessage.addListener(async message => {
    if (message.op === 'abort') { controller?.abort(); return; }
    if (message.op !== 'chat' && message.op !== 'brief') return;
    controller?.abort();
    controller = new AbortController();
    const {signal} = controller;
    const requestId = message.requestId;
    const send = payload => { try { port.postMessage({requestId, ...payload}); } catch { /* page gone */ } };
    let state = createAssistantState();
    send({op: 'view', view: toView(state)});
    const accessToken = await readToken(session);
    if (!accessToken) { send({op: 'signed-out'}); return; }
    const onEvent = event => { state = applyStreamEvent(state, event); send({op: 'view', view: toView(state)}); };
    try {
      if (message.op === 'brief') {
        const result = await streamBrief({baseUrl: backend, token: accessToken, signal, onEvent});
        if (result.transport === 'unsupported') { send({op: 'unsupported'}); return; }
      } else {
        const cid = await conversationId();
        await streamChat({
          baseUrl: backend, token: accessToken, signal, onEvent,
          request: {
            message: message.message,
            ...(cid ? {conversation_id: cid} : {}),
            ...(message.pageContext ? {page_context: message.pageContext} : {}),
          },
          // The character needs streaming + cards; without /api/chat/stream it reports the backend as unavailable.
          legacyChat: async () => { throw Object.assign(new Error('stream endpoint missing'), {status: 503}); },
        });
      }
      state = finalizeStream(state);
      if (message.op === 'chat' && state.conversationId) await session.set({conversationId: state.conversationId});
    } catch (error) {
      if (error?.status === 401) { send({op: 'signed-out'}); return; }
      state = finalizeStream(state, signal.aborted ? 'interrupted' : failureCode(error));
    }
    send({op: 'view', view: toView(state)});
  });
});

async function handle(message, sender) {
  const accessToken = await readToken(session);
  const options = {baseUrl: backend, token: accessToken};
  switch (message.op) {
    case 'session': {
      if (!accessToken) return {authenticated: false};
      try {
        const me = await getMe(backend, accessToken);
        return {authenticated: true, user: {id: me.id, name: displayName(me)}}; // the page gets no more than this
      } catch (error) {
        return {authenticated: false, unavailable: error?.status !== 401};
      }
    }
    case 'login': {
      try {
        const result = await login(backend, message.email, message.password);
        await saveToken(session, result.access_token);
        await session.remove('conversationId');
        return {ok: true};
      } catch (error) {
        return {ok: false, error: error?.status === 401 ? 'Неверный email или пароль.' : 'Backend недоступен.'};
      }
    }
    case 'confirm':
    case 'cancel': {
      try {
        if (message.op === 'confirm') return describeResult(await confirmAction(options, message.actionId));
        await cancelAction(options, message.actionId);
        return {tone: 'muted', text: 'Отменено. Заявка не отправлена.'};
      } catch (error) {
        const {text, retryable} = describeActionError(error?.status, message.op);
        return {tone: 'error', text, retryable};
      }
    }
    case 'feedback': {
      try { await sendFeedback(options, message.messageId, message.rating); return {ok: true}; } catch { return {ok: false}; }
    }
    case 'new-dialog':
      await session.remove('conversationId');
      return {ok: true};
    case 'requests': {
      if (!accessToken) return {changes: []};
      try {
        const response = await fetch(`${backend}/api/requests`, {
          headers: {Authorization: `Bearer ${accessToken}`}, signal: AbortSignal.timeout(10000),
        });
        if (!response.ok) return {changes: []};
        const stored = await chrome.storage.local.get('buddyRequests');
        const {changes, next} = requestStatusChanges(stored.buddyRequests, await response.json());
        await chrome.storage.local.set({buddyRequests: next});
        return {changes};
      } catch { return {changes: []}; }
    }
    case 'open-panel':
      // A click in the page is a user gesture that Chrome forwards with the message.
      await chrome.sidePanel.open({tabId: sender.tab.id});
      return {ok: true};
    case 'hint':
      return {hint: pageHint({title: String(message.title || ''), url: String(message.url || '')})};
    case 'site-state':
      return {enabled: !(await offSites()).includes(message.pattern)};
    case 'set-site':
      await setSite(message.pattern, message.enabled, message.tabId);
      return {ok: true};
    default:
      return {error: 'unknown op'};
  }
}

// Only our own pages and content scripts can reach this listener (no externally_connectable).
chrome.runtime.onMessage.addListener((message, sender, reply) => {
  handle(message, sender).then(reply, error => reply({error: String(error?.message || error)}));
  return true; // async reply
});
