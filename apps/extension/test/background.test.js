// background.js (MV3 service worker): content script registration, the toolbar icon, message routing to the
// gateway, single sign-on with the web app and the streaming port. chrome.* is a scripted in-memory fake;
// './client.js' (copied from packages/client at build time) resolves to test/fixtures/fake-client.js.
import assert from 'node:assert/strict';
import { register } from 'node:module';
import { mock, test } from 'node:test';
import { jsonResponse, sseResponse } from './helpers.js';

register('./fixtures/client-hooks.mjs', import.meta.url);

const flush = async (rounds = 10) => { for (let i = 0; i < rounds; i += 1) await new Promise(resolve => setImmediate(resolve)); };
const b64 = value => Buffer.from(JSON.stringify(value)).toString('base64url');
const jwt = sub => `${b64({alg: 'HS256'})}.${b64({sub})}.sig`;
const pair = (sub, refresh = `r-${sub}`) => ({access_token: jwt(sub), refresh_token: refresh, expires_in: 900});

function event() {
  const listeners = [];
  return {listeners, addListener: fn => listeners.push(fn), emit: (...args) => listeners.map(fn => fn(...args))};
}
function area() {
  const data = {};
  return {
    data,
    get: async key => (key in data ? {[key]: data[key]} : {}),
    set: async obj => { Object.assign(data, obj); },
    remove: async key => { delete data[key]; },
  };
}

const log = [];
const record = name => async (...args) => { log.push([name, ...args]); };
const chrome = {
  action: {onClicked: event(), setBadgeBackgroundColor: record('badgeColor'), setBadgeText: record('badgeText'), setTitle: record('title')},
  storage: {local: area(), session: area()},
  tabs: {
    sendMessage: async (tabId, message) => { log.push(['sendMessage', tabId, message]); },
    query: async query => { log.push(['query', query]); return [{id: 41}, {id: 42}]; },
  },
  scripting: {
    registered: [],
    executeScript: record('executeScript'),
    getRegisteredContentScripts: async ({ids}) => chrome.scripting.registered.filter(s => ids.includes(s.id)),
    registerContentScripts: async scripts => { log.push(['register', scripts]); chrome.scripting.registered.push(...scripts); },
    updateContentScripts: record('update'),
  },
  runtime: {onInstalled: event(), onStartup: event(), onConnect: event(), onMessage: event(), getURL: path => `chrome-extension://abc/${path}`},
};
globalThis.chrome = chrome;

const client = {
  calls: [],
  login: async (_base, email) => { client.calls.push(['login', email]); if (email === 'bad') throw Object.assign(new Error('401'), {status: 401}); return {...pair('u1'), user: {id: 'u1'}}; },
  getMe: async (_base, token) => { client.calls.push(['getMe', token]); return {id: 'u1', first_name: 'Аня', last_name: 'Петрова', email: 'a@x'}; },
  linkSession: async (_base, refresh) => { client.calls.push(['link', refresh]); return pair('u1', 'linked'); },
  logoutSession: async (_base, _access, refresh) => { client.calls.push(['logout', refresh]); },
  refreshSession: async () => pair('u1'),
};
globalThis.__fakeClient = client;
// the gateway captures fetch when the worker starts: route it to whatever the current test scripts
let fetchImpl = async url => { throw new Error(`unexpected fetch ${url}`); };
globalThis.fetch = (...args) => fetchImpl(...args);

await import('../background.js');

const SITE = {tab: {id: 7}, url: 'http://127.0.0.1:5173/dashboard'};
function send(message, sender = {tab: {id: 1}, url: 'https://wiki.example.com/'}) {
  return new Promise(resolve => {
    const [async] = chrome.runtime.onMessage.emit(message, sender, resolve);
    assert.equal(async, true, 'the listener answers asynchronously');
  });
}

test('the character script is registered on install and updated on startup', async () => {
  chrome.runtime.onInstalled.emit();
  await flush();
  const [, [script]] = log.find(e => e[0] === 'register');
  assert.deepEqual(script, {
    id: 'ai-buddy-character', js: ['buddy/bridge.js', 'buddy/widget.js', 'buddy/content.js'],
    matches: ['http://*/*', 'https://*/*'], runAt: 'document_idle', persistAcrossSessions: true,
  });
  chrome.runtime.onStartup.emit();
  await flush();
  assert.equal(log.at(-1)[0], 'update');
});

test('toolbar icon on an ordinary page: shown everywhere, toggles the chat on this tab', async () => {
  log.length = 0;
  chrome.action.onClicked.emit({url: 'https://x'}); // no tab id
  chrome.action.onClicked.emit({id: 3, url: 'https://wiki.example.com/'});
  await flush();
  assert.equal(chrome.storage.local.data.buddyVisible, true);
  assert.deepEqual(log, [['sendMessage', 3, {op: 'toggle', wasShown: true}]]);

  chrome.storage.local.data.buddyVisible = false;
  log.length = 0;
  chrome.action.onClicked.emit({id: 3, url: 'https://wiki.example.com/'});
  await flush();
  assert.deepEqual(log, [['sendMessage', 3, {op: 'toggle', wasShown: false}]]);
  assert.equal(chrome.storage.local.data.buddyVisible, true);
});

test('toolbar icon on a tab opened before install: the scripts are injected first', async () => {
  const sendMessage = chrome.tabs.sendMessage;
  let fails = 1;
  chrome.tabs.sendMessage = async (tabId, message) => {
    if (fails-- > 0) throw new Error('Could not establish connection');
    log.push(['sendMessage', tabId, message]);
  };
  log.length = 0;
  chrome.action.onClicked.emit({id: 5, url: 'http://intranet/'});
  await flush();
  assert.deepEqual(log[0], ['executeScript', {target: {tabId: 5}, files: ['buddy/bridge.js', 'buddy/widget.js', 'buddy/content.js']}]);
  assert.equal(log[1][0], 'sendMessage');

  chrome.tabs.sendMessage = async () => { throw new Error('nope'); };
  const errors = [];
  const consoleError = console.error;
  console.error = (...args) => errors.push(args);
  chrome.action.onClicked.emit({id: 6, url: 'http://intranet/'});
  await flush();
  console.error = consoleError;
  assert.equal(errors.length, 1);
  assert.match(errors[0][0], /cannot show Бади/);
  chrome.tabs.sendMessage = sendMessage;
});

test('toolbar icon on chrome:// pages: a badge says Бади is shown on ordinary pages', async () => {
  mock.timers.enable({apis: ['setTimeout']});
  log.length = 0;
  chrome.storage.local.data.buddyVisible = false;
  chrome.action.onClicked.emit({id: 9, url: 'chrome://extensions'});
  await flush();
  assert.equal(chrome.storage.local.data.buddyVisible, true);
  assert.deepEqual(log.map(e => e[0]), ['badgeColor', 'badgeText', 'title']);
  assert.deepEqual(log[1][1], {text: '✓', tabId: 9});
  mock.timers.tick(4000);
  await flush();
  assert.deepEqual(log.slice(3), [['badgeText', {text: '', tabId: 9}], ['title', {title: 'Открыть AI-buddy', tabId: 9}]]);

  // the tab was closed: no error escapes
  const setBadgeText = chrome.action.setBadgeText;
  chrome.action.setBadgeText = async () => { throw new Error('No tab with id'); };
  chrome.action.onClicked.emit({id: 10, url: 'chrome://newtab'});
  await flush();
  chrome.action.setBadgeText = setBadgeText;
  mock.timers.reset();
});

test('messages go to the gateway: login stores the session and tells the open site tabs', async () => {
  assert.deepEqual(await send({op: 'session'}), {authenticated: false});
  log.length = 0;
  chrome.storage.session.data.conversationId = 'old';
  assert.deepEqual(await send({op: 'login', email: 'a@x', password: 'pw'}), {ok: true});
  await flush();
  assert.equal(chrome.storage.local.data.auth.sub, 'u1');
  assert.equal(chrome.storage.session.data.conversationId, undefined, 'the conversation lives in chrome.storage.session');
  assert.deepEqual(log.find(e => e[0] === 'query'), ['query', {url: ['http://127.0.0.1:5173/*', 'http://localhost:5173/*']}]);
  assert.deepEqual(log.filter(e => e[0] === 'sendMessage').map(e => [e[1], e[2]]), [[41, {op: 'sso-changed'}], [42, {op: 'sso-changed'}]]);

  const session = await send({op: 'session'});
  assert.equal(session.authenticated, true);
  assert.equal(session.user.id, 'u1');
  assert.deepEqual(await send({op: 'login', email: 'bad', password: 'pw'}), {ok: false, error: 'Неверный email или пароль.'});
  assert.deepEqual(await send({op: 'nope'}), {error: 'unknown op'});
});

test('request statuses are remembered in chrome.storage.local', async () => {
  fetchImpl = async url => jsonResponse(String(url).includes('corporate') ? [] : [{id: 'r1', number: 'REQ-1', status: 'approved', type: 'leave_request'}]);
  assert.deepEqual(await send({op: 'requests'}), {changes: []});
  assert.deepEqual(chrome.storage.local.data.buddyRequests, {'REQ-1': 'approved'});
});

test('errors thrown while handling a message are answered, not lost', async () => {
  const get = chrome.storage.local.get;
  chrome.storage.local.get = async () => { throw new Error('storage broken'); };
  assert.deepEqual(await send({op: 'session'}), {error: 'storage broken'});
  chrome.storage.local.get = get;
});

test('single sign-on is only for the web app tabs', async () => {
  const other = {tab: {id: 1}, url: 'https://evil.example/'};
  assert.deepEqual(await send({op: 'sso-handshake', state: {}}, other), {error: 'forbidden'});
  assert.deepEqual(await send({op: 'sso-handshake', state: {}}, {url: 'http://127.0.0.1:5173/'}), {error: 'forbidden'}, 'not a tab');
  assert.deepEqual(await send({op: 'sso-handshake', state: {}}, {tab: {id: 1}, url: 'not a url'}), {error: 'forbidden'});
  assert.deepEqual(await send({op: 'sso-adopt', pair: pair('u2')}, other), {error: 'forbidden'});
  assert.deepEqual(await send({op: 'sso-adopt', pair: {access_token: 1}}, SITE), {error: 'forbidden'});
  assert.deepEqual(await send({op: 'sso-adopt'}, SITE), {error: 'forbidden'});
  assert.deepEqual(await send({op: 'sso-logout'}, other), {error: 'forbidden'});
});

test('handshake: the signed-in extension links a session for the site; same user does nothing', async () => {
  client.calls.length = 0;
  const pulled = await send({op: 'sso-handshake', state: {}}, SITE);
  assert.equal(pulled.action, 'pull');
  assert.equal(pulled.pair.refresh_token, 'linked');
  assert.ok(pulled.loginAt > 0);
  assert.deepEqual(client.calls.find(c => c[0] === 'link'), ['link', 'r-u1']);

  assert.deepEqual(await send({op: 'sso-handshake', state: {sub: 'u1', loginAt: 1}}, {tab: {id: 2}, url: 'http://localhost:5173/x'}), {action: 'none'});
  assert.deepEqual(await send({op: 'sso-handshake', state: {sub: 'u9', loginAt: Date.now() + 60_000}}, SITE), {action: 'push'});

  // linking fails: nothing to pull
  const link = client.linkSession;
  client.linkSession = async () => { throw new Error('offline'); };
  assert.deepEqual(await send({op: 'sso-handshake', state: {}}, SITE), {action: 'none'});
  client.linkSession = link;
});

test('adopting the site session of another user starts a new dialog', async () => {
  chrome.storage.session.data.conversationId = 'c1';
  assert.deepEqual(await send({op: 'sso-adopt', pair: pair('u1', 'site-1'), loginAt: 5}, SITE), {ok: true});
  assert.equal(chrome.storage.session.data.conversationId, 'c1', 'same user: the dialog goes on');
  assert.equal(chrome.storage.local.data.auth.loginAt, 5);
  assert.deepEqual(await send({op: 'sso-adopt', pair: pair('u2', 'site-2')}, SITE), {ok: true});
  assert.equal(chrome.storage.local.data.auth.sub, 'u2');
  assert.equal(chrome.storage.session.data.conversationId, undefined);
  assert.ok(chrome.storage.local.data.auth.loginAt > 5, 'no loginAt: now');
});

test('the site signed out after our sign-in: the extension follows; sso-logout signs out too', async () => {
  chrome.storage.session.data.conversationId = 'c2';
  const ext = chrome.storage.local.data.auth;
  assert.deepEqual(await send({op: 'sso-handshake', state: {signedOutAt: ext.loginAt + 1}}, SITE), {action: 'none'});
  assert.equal(chrome.storage.local.data.auth, undefined);
  assert.equal(chrome.storage.session.data.conversationId, undefined);

  await send({op: 'sso-adopt', pair: pair('u1', 'again')}, SITE);
  chrome.storage.session.data.conversationId = 'c3';
  assert.deepEqual(await send({op: 'sso-logout'}, SITE), {ok: true});
  assert.equal(chrome.storage.local.data.auth, undefined);
  assert.equal(chrome.storage.session.data.conversationId, undefined);
  assert.ok(chrome.storage.local.data.signedOutAt > 0);
  assert.deepEqual(await send({op: 'logout'}), {ok: true});
});

function connect(name = 'buddy') {
  const port = {name, posted: [], onMessage: event(), onDisconnect: event(), postMessage(m) { if (port.broken) throw new Error('gone'); port.posted.push(m); }};
  chrome.runtime.onConnect.emit(port);
  return port;
}

test('the buddy port streams answers as view models tagged with the request id', async () => {
  await send({op: 'sso-adopt', pair: pair('u1', 'stream')}, SITE);
  fetchImpl = async () => sseResponse([['meta', {conversation_id: 'c9', message_id: 'm9'}], ['delta', {text: 'Ответ'}], ['done', {}]]);
  const other = connect('devtools');
  assert.equal(other.onMessage.listeners.length, 0, 'other ports are ignored');

  const port = connect();
  port.onMessage.emit({op: 'session'}); // only chat/brief stream here
  port.onMessage.emit({op: 'chat', message: 'Привет', requestId: 1});
  await flush(30);
  const last = port.posted.at(-1);
  assert.equal(last.requestId, 1);
  assert.equal(last.op, 'view');
  assert.equal(last.view.text, 'Ответ');
  assert.equal(last.view.finished, true);
  assert.equal(chrome.storage.session.data.conversationId, 'c9');
});

test('a new question, "abort" and a closed page abort the running answer', async () => {
  const signals = [];
  fetchImpl = (_url, init) => new Promise((_resolve, reject) => {
    signals.push(init.signal);
    init.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), {name: 'AbortError'})));
  });
  const port = connect();
  port.onMessage.emit({op: 'chat', message: 'один', requestId: 1});
  await flush();
  port.onMessage.emit({op: 'brief', requestId: 2});
  await flush();
  assert.equal(signals[0].aborted, true, 'the second request aborts the first');
  port.onMessage.emit({op: 'abort'});
  await flush();
  assert.equal(signals[1].aborted, true);
  const interrupted = port.posted.filter(m => m.view?.error === 'Соединение прервалось до конца ответа.');
  assert.deepEqual(interrupted.map(m => m.requestId), [1, 2]);

  port.onMessage.emit({op: 'chat', message: 'три', requestId: 3});
  await flush();
  port.broken = true; // the page is gone: posting fails quietly
  port.onDisconnect.emit();
  await flush();
  assert.equal(signals[2].aborted, true);

  const idle = connect();
  idle.onDisconnect.emit();
  idle.onMessage.emit({op: 'abort'});
});
