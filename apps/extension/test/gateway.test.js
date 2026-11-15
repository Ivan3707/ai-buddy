import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createGateway } from '../lib/gateway.js';
import { fakeFetch, jsonResponse, sseResponse, textResponse } from './helpers.js';

const base = 'http://api.test';

// Fake session core (lib/auth.js shape): token() returns the current token, force rotates it once.
function fakeAuth({token = 'tok', rotated = 'tok2'} = {}) {
  const auth = {
    current: token,
    calls: [],
    async token(opts = {}) {
      auth.calls.push(['token', opts]);
      if (opts.force) { if (rotated && auth.current !== rotated) auth.current = rotated; else return opts.stale; }
      return auth.current;
    },
    loginImpl: async () => {},
    async login(email, password) { auth.calls.push(['login', email, password]); return auth.loginImpl(email, password); },
    async logout() { auth.calls.push(['logout']); auth.current = ''; },
  };
  return auth;
}

function memoryStore(initial = {}) {
  const data = new Map(Object.entries(initial));
  return {
    data,
    get: async key => data.get(key),
    set: async (key, value) => { data.set(key, value); },
    remove: async key => { data.delete(key); },
  };
}

function setup({responses = [], auth = fakeAuth(), store = memoryStore(), client = {}, hooks = {}} = {}) {
  const fetchImpl = fakeFetch(...responses);
  const gateway = createGateway({baseUrl: base, auth, hooks, store, client, fetchImpl});
  return {gateway, fetchImpl, auth, store};
}

const httpError = status => Object.assign(new Error(`HTTP ${status}`), {status});

test('session: signed out without a token; user name on success; retries getMe once after 401', async () => {
  const signedOut = setup({auth: fakeAuth({token: ''})});
  assert.deepEqual(await signedOut.gateway.call({op: 'session'}), {authenticated: false});

  const seen = [];
  const client = {getMe: async (_b, token) => { seen.push(token); if (token === 'tok') throw httpError(401); return {id: 'u1', first_name: 'Аня', email: 'a@x'}; }};
  const {gateway} = setup({client});
  const state = await gateway.call({op: 'session'});
  assert.equal(state.authenticated, true);
  assert.equal(state.user.id, 'u1');
  assert.ok(state.user.name);
  assert.deepEqual(seen, ['tok', 'tok2']);
});

test('session: 401 without a new token signs out; other errors mean "backend unavailable"', async () => {
  const auth = fakeAuth({rotated: null});
  const r401 = setup({auth, client: {getMe: async () => { throw httpError(401); }}});
  assert.deepEqual(await r401.gateway.call({op: 'session'}), {authenticated: false, unavailable: false});

  const down = setup({client: {getMe: async () => { throw new TypeError('fetch failed'); }}});
  assert.deepEqual(await down.gateway.call({op: 'session'}), {authenticated: false, unavailable: true});
});

test('login drops the old conversation and calls the hook; wrong password and outages are told apart', async () => {
  let loggedIn = 0;
  const store = memoryStore({conversationId: 'c-old'});
  const {gateway, auth} = setup({store, hooks: {onLogin: () => { loggedIn += 1; }}});
  assert.deepEqual(await gateway.call({op: 'login', email: 'a@x', password: 'pw'}), {ok: true});
  assert.equal(store.data.has('conversationId'), false);
  assert.equal(loggedIn, 1);
  assert.deepEqual(auth.calls.find(c => c[0] === 'login'), ['login', 'a@x', 'pw']);

  auth.loginImpl = async () => { throw httpError(401); };
  assert.deepEqual(await gateway.call({op: 'login', email: 'a', password: 'b'}), {ok: false, error: 'Неверный email или пароль.'});
  auth.loginImpl = async () => { throw new TypeError('offline'); };
  assert.deepEqual(await gateway.call({op: 'login', email: 'a', password: 'b'}), {ok: false, error: 'Backend недоступен.'});
});

test('logout and new-dialog forget the conversation', async () => {
  let out = 0;
  const store = memoryStore({conversationId: 'c1'});
  const {gateway, auth} = setup({store, hooks: {onLogout: () => { out += 1; }}});
  assert.deepEqual(await gateway.call({op: 'logout'}), {ok: true});
  assert.equal(out, 1);
  assert.ok(auth.calls.some(c => c[0] === 'logout'));
  assert.equal(store.data.has('conversationId'), false);

  store.data.set('conversationId', 'c2');
  assert.deepEqual(await gateway.call({op: 'new-dialog'}), {ok: true});
  assert.equal(store.data.has('conversationId'), false);
  // works without hooks too
  const plain = setup();
  assert.deepEqual(await plain.gateway.call({op: 'logout'}), {ok: true});
});

test('confirm / cancel describe the outcome and the HTTP errors', async () => {
  const {gateway, fetchImpl} = setup({responses: [
    jsonResponse({request: {id: 'r1', number: 'REQ-1', status: 'pending_approval', approver: 'Иванов'}}),
    new Response(null, {status: 204}),
    jsonResponse({error: 'gone'}, 409),
    jsonResponse({error: 'boom'}, 500),
  ]});
  const ok = await gateway.call({op: 'confirm', actionId: 'a 1'});
  assert.equal(ok.tone, 'success');
  assert.match(ok.text, /REQ-1/);
  assert.equal(ok.requestId, 'r1');
  assert.equal(fetchImpl.calls[0].url, `${base}/api/actions/a%201/confirm`);
  assert.equal(fetchImpl.calls[0].init.headers.Authorization, 'Bearer tok');

  assert.deepEqual(await gateway.call({op: 'cancel', actionId: 'a1'}), {tone: 'muted', text: 'Отменено. Заявка не отправлена.'});
  assert.equal(fetchImpl.calls[1].url, `${base}/api/actions/a1/cancel`);

  const conflict = await gateway.call({op: 'cancel', actionId: 'a1'});
  assert.equal(conflict.tone, 'error');
  assert.equal(conflict.retryable, false);
  const failed = await gateway.call({op: 'confirm', actionId: 'a1'});
  assert.equal(failed.retryable, true);
});

test('feedback reports ok / not ok', async () => {
  const {gateway, fetchImpl} = setup({responses: [new Response(null, {status: 204}), jsonResponse({error: 'x'}, 500)]});
  assert.deepEqual(await gateway.call({op: 'feedback', messageId: 'm1', rating: 'down', comment: 'плохо'}), {ok: true});
  assert.deepEqual(fetchImpl.calls[0].body, {rating: 'down', comment: 'плохо'});
  assert.deepEqual(await gateway.call({op: 'feedback', messageId: 'm1', rating: 'up'}), {ok: false});
});

test('requests: merges own and corporate requests, remembers statuses and reports changes', async () => {
  const own = [{id: 'r1', number: 'REQ-1', status: 'pending_approval', type: 'leave_request', created_at: '2026-01-01'}];
  const corporate = [{id: 'c1', ticket_number: 'ACC-1', status: 'open', tool: 'jira', created_at: '2026-01-02'}];
  const store = memoryStore();
  const {gateway, fetchImpl} = setup({store, responses: [
    jsonResponse(own), jsonResponse(corporate),
    jsonResponse([{...own[0], status: 'approved'}]), jsonResponse([{...corporate[0], status: 'denied'}]),
  ]});
  assert.deepEqual(await gateway.call({op: 'requests'}), {changes: []});
  assert.deepEqual(store.data.get('buddyRequests'), {'REQ-1': 'pending_approval', 'ACC-1': 'pending_approval'});
  assert.deepEqual(fetchImpl.calls.map(c => c.url).slice(0, 2), [`${base}/api/requests`, `${base}/api/corporate/access-requests`]);
  const {changes} = await gateway.call({op: 'requests'});
  assert.deepEqual(changes.map(c => c.text), ['Заявка ACC-1: отклонена', 'Заявка REQ-1: согласована']);

  assert.deepEqual(await setup({auth: fakeAuth({token: ''})}).gateway.call({op: 'requests'}), {changes: []});
  const broken = setup({responses: [jsonResponse({}, 500), jsonResponse({}, 500)]});
  assert.deepEqual(await broken.gateway.call({op: 'requests'}), {changes: []});
});

test('requests-list: one side may fail; both failing reports the error; unknown corporate status is pending', async () => {
  const {gateway} = setup({responses: [
    jsonResponse({}, 500), jsonResponse([{id: 'c1', ticket_number: 'ACC-9', status: 'weird', tool: 'grafana', created_at: 'x'}]),
    jsonResponse([{id: 'r1', number: 'REQ-2', status: 'approved', type: 'access_request', params: null, approver: 'Петров', created_at: 'b'}]), jsonResponse({}, 503),
    jsonResponse({}, 500), jsonResponse({}, 500),
  ]});
  const one = await gateway.call({op: 'requests-list'});
  assert.deepEqual(one.requests, [{kind: 'corporate', id: 'c1', number: 'ACC-9', status: 'pending_approval', type: 'corporate_access', params: {tool: 'grafana'}, approver: '', created_at: 'x'}]);
  const two = await gateway.call({op: 'requests-list'});
  assert.deepEqual(two.requests[0], {kind: 'assistant', id: 'r1', number: 'REQ-2', status: 'approved', type: 'access_request', params: {}, approver: 'Петров', created_at: 'b'});
  assert.deepEqual(await gateway.call({op: 'requests-list'}), {error: 'HTTP 500'});
});

test('getJson retries once with a rotated token after 401', async () => {
  const {gateway, fetchImpl} = setup({responses: [jsonResponse({}, 401), jsonResponse([{id: 'n1'}])]});
  assert.deepEqual(await gateway.call({op: 'notes'}), {notes: [{id: 'n1'}]});
  assert.equal(fetchImpl.calls[0].init.headers.Authorization, 'Bearer tok');
  assert.equal(fetchImpl.calls[1].init.headers.Authorization, 'Bearer tok2');
});

test('a 401 that survives the retry becomes "session expired"', async () => {
  const {gateway} = setup({auth: fakeAuth({rotated: null}), responses: [jsonResponse({}, 401)]});
  assert.deepEqual(await gateway.call({op: 'notes'}), {error: 'Сессия истекла. Обновите страницу.'});
});

test('hint uses the page title and url', async () => {
  const {gateway} = setup();
  assert.match((await gateway.call({op: 'hint', title: 'Grafana', url: 'https://g'})).hint.text, /дашборд/);
  assert.deepEqual(await gateway.call({op: 'hint'}), {hint: null});
});

test('doc: validates the id, maps 404 and other failures', async () => {
  const {gateway, fetchImpl} = setup({responses: [jsonResponse({id: 'kb-1', title: 'Док'}), jsonResponse({}, 404), new TypeError('offline')]});
  assert.deepEqual(await gateway.call({op: 'doc', id: '../etc'}), {error: 'Документ не найден.'});
  assert.deepEqual(await gateway.call({op: 'doc', id: 'kb-1'}), {doc: {id: 'kb-1', title: 'Док'}});
  assert.equal(fetchImpl.calls[0].url, `${base}/api/kb/docs/kb-1`);
  assert.deepEqual(await gateway.call({op: 'doc', id: 'kb-2'}), {error: 'Документ не найден в базе знаний.'});
  assert.deepEqual(await gateway.call({op: 'doc', id: 'kb-3'}), {error: 'Не удалось загрузить документ.'});
});

test('notes: load, create, update and validation', async () => {
  const {gateway, fetchImpl} = setup({responses: [
    new TypeError('offline'),
    jsonResponse({id: 'n1', title: 'T'}, 201),
    jsonResponse({id: 'n1', title: 'T2'}),
    textResponse('oops', 500),
    jsonResponse({error: 'слишком длинно'}, 400),
  ]});
  assert.deepEqual(await gateway.call({op: 'notes'}), {error: 'Не удалось загрузить заметки.'});
  assert.deepEqual(await gateway.call({op: 'note-save', title: '   '}), {error: 'Введите заголовок.'});
  assert.deepEqual(await gateway.call({op: 'note-save', title: ' T ', content: 'body'}), {note: {id: 'n1', title: 'T'}});
  assert.equal(fetchImpl.calls[1].init.method, 'POST');
  assert.deepEqual(fetchImpl.calls[1].body, {title: 'T', content: 'body'});
  assert.deepEqual(await gateway.call({op: 'note-save', id: 'n1', title: 'T2'}), {note: {id: 'n1', title: 'T2'}});
  assert.equal(fetchImpl.calls[2].init.method, 'PUT');
  assert.equal(fetchImpl.calls[2].url, `${base}/api/notes/n1`);
  assert.deepEqual(await gateway.call({op: 'note-save', title: 'x'}), {error: 'HTTP 500'});
  assert.deepEqual(await gateway.call({op: 'note-save', title: 'x'}), {error: 'слишком длинно'});
});

test('request-create keeps only string params and picks the type', async () => {
  const {gateway, fetchImpl} = setup({responses: [jsonResponse({id: 'r1', number: 'REQ-1'}), jsonResponse({id: 'r2', number: 'REQ-2'}), new TypeError('x')]});
  assert.deepEqual(await gateway.call({op: 'request-create', type: 'access_request', params: {system: 'jira', n: 5, resource: 'x'.repeat(600)}}),
    {request: {id: 'r1', number: 'REQ-1'}});
  assert.deepEqual(fetchImpl.calls[0].body, {type: 'access_request', params: {system: 'jira', resource: 'x'.repeat(500)}});
  await gateway.call({op: 'request-create', type: 'whatever'});
  assert.deepEqual(fetchImpl.calls[1].body, {type: 'leave_request', params: {}});
  assert.deepEqual(await gateway.call({op: 'request-create'}), {error: 'Не удалось создать заявку.'});
});

test('suggestions: questions as strings; empty on failure', async () => {
  const {gateway} = setup({responses: [jsonResponse({suggestions: [{question: 'Где VPN?'}, {question: ''}]}), jsonResponse({}), jsonResponse({}, 500)]});
  assert.deepEqual(await gateway.call({op: 'suggestions'}), {suggestions: ['Где VPN?']});
  assert.deepEqual(await gateway.call({op: 'suggestions'}), {suggestions: []});
  assert.deepEqual(await gateway.call({op: 'suggestions'}), {suggestions: []});
});

test('hint-turn sends the conversation and remembers the new one', async () => {
  const store = memoryStore({conversationId: 'c1'});
  const {gateway, fetchImpl} = setup({store, responses: [
    jsonResponse({text: 'Ответ', conversation_id: 'c2'}), jsonResponse({text: 'Ещё'}), jsonResponse({error: 'nope'}, 500),
  ]});
  assert.deepEqual(await gateway.call({op: 'hint-turn', question: '  Где VPN? '}), {hint: {text: 'Ответ', conversation_id: 'c2'}});
  assert.deepEqual(fetchImpl.calls[0].body, {question: 'Где VPN?', conversation_id: 'c1'});
  assert.equal(store.data.get('conversationId'), 'c2');
  store.data.delete('conversationId');
  await gateway.call({op: 'hint-turn'});
  assert.deepEqual(fetchImpl.calls[1].body, {question: ''});
  assert.deepEqual(await gateway.call({op: 'hint-turn', question: 'x'}), {error: 'nope'});
});

test('questions and question-create', async () => {
  const {gateway, fetchImpl} = setup({responses: [jsonResponse([{id: 'q1'}]), jsonResponse({}, 500), jsonResponse({id: 'q2'}), new TypeError('x')]});
  assert.deepEqual(await gateway.call({op: 'questions'}), {questions: [{id: 'q1'}]});
  assert.deepEqual(await gateway.call({op: 'questions'}), {error: 'HTTP 500'});
  assert.deepEqual(await gateway.call({op: 'question-create', question: '  '}), {error: 'Напишите вопрос.'});
  assert.deepEqual(await gateway.call({op: 'question-create', question: ' Как? '}), {question: {id: 'q2'}});
  assert.deepEqual(fetchImpl.calls[2].body, {question: 'Как?'});
  assert.deepEqual(await gateway.call({op: 'question-create', question: 'x'}), {error: 'Не удалось отправить вопрос.'});
});

test('events and event-create validate the title and the time range', async () => {
  const {gateway, fetchImpl} = setup({responses: [jsonResponse([]), new TypeError('x'), jsonResponse({id: 'e1'}), new TypeError('x')]});
  assert.deepEqual(await gateway.call({op: 'events'}), {events: []});
  assert.deepEqual(await gateway.call({op: 'events'}), {error: 'Не удалось загрузить календарь.'});
  assert.deepEqual(await gateway.call({op: 'event-create', title: ''}), {error: 'Введите название.'});
  assert.deepEqual(await gateway.call({op: 'event-create', title: 'Встреча', starts_at: '2026-01-01T10:00', ends_at: '2026-01-01T09:00'}),
    {error: 'Окончание должно быть позже начала.'});
  assert.deepEqual(await gateway.call({op: 'event-create', title: 'Встреча', starts_at: '2026-01-01T10:00:00Z', ends_at: '2026-01-01T11:00:00Z'}), {event: {id: 'e1'}});
  assert.deepEqual(fetchImpl.calls[2].body, {title: 'Встреча', description: '', starts_at: '2026-01-01T10:00:00.000Z', ends_at: '2026-01-01T11:00:00.000Z'});
  assert.deepEqual(await gateway.call({op: 'event-create', title: 'X', starts_at: '2026-01-01T10:00:00Z', ends_at: '2026-01-01T11:00:00Z'}),
    {error: 'Не удалось создать событие.'});
});

test('unknown ops are rejected', async () => {
  const {gateway} = setup();
  assert.deepEqual(await gateway.call({op: 'nope'}), {error: 'unknown op'});
  assert.deepEqual(await gateway.call(undefined), {error: 'unknown op'});
});

// ---------- streaming ----------

function collect(gateway, payload) {
  const messages = [];
  let resolve;
  const done = new Promise(r => { resolve = r; });
  const abort = gateway.stream(payload, message => {
    messages.push(message);
    if (message.op !== 'view' || message.view.finished) resolve();
  });
  return {messages, done, abort};
}

test('stream chat: views follow the SSE events, the conversation is remembered and page context sent', async () => {
  const store = memoryStore({conversationId: 'c0'});
  const {gateway, fetchImpl} = setup({store, responses: [sseResponse([
    ['meta', {conversation_id: 'c1', message_id: 'm1'}],
    ['status', {stage: 'generating'}],
    ['delta', {text: 'При'}],
    ['delta', {text: 'вет'}],
    ['sources', {sources: [{id: 'kb-1', title: 'VPN', url: 'https://wiki/vpn', snippet: 's'}]}],
    ['done', {finish_reason: 'answered'}],
  ])]});
  const run = collect(gateway, {op: 'chat', message: 'Как VPN?', pageContext: {url: 'https://x'}});
  await run.done;
  const last = run.messages.at(-1).view;
  assert.equal(last.text, 'Привет');
  assert.equal(last.messageId, 'm1');
  assert.equal(last.sources[0].title, 'VPN');
  assert.equal(run.messages[0].view.finished, false);
  assert.deepEqual(fetchImpl.calls[0].body, {message: 'Как VPN?', conversation_id: 'c0', page_context: {url: 'https://x'}});
  await new Promise(r => setImmediate(r));
  assert.equal(store.data.get('conversationId'), 'c1');
});

test('stream: signed out without a token or on 401', async () => {
  const noToken = collect(setup({auth: fakeAuth({token: ''})}).gateway, {op: 'chat', message: 'x'});
  await noToken.done;
  assert.equal(noToken.messages.at(-1).op, 'signed-out');

  const r401 = collect(setup({responses: [jsonResponse({error: 'no'}, 401)]}).gateway, {op: 'chat', message: 'x'});
  await r401.done;
  assert.equal(r401.messages.at(-1).op, 'signed-out');
});

test('stream: backend failures become an error view; a missing stream endpoint counts as unavailable', async () => {
  const failed = collect(setup({responses: [jsonResponse({error: 'down'}, 503)]}).gateway, {op: 'chat', message: 'x'});
  await failed.done;
  assert.equal(failed.messages.at(-1).view.error, 'AI-сервис временно недоступен.');

  const legacy = collect(setup({responses: [textResponse('404 page not found', 404)]}).gateway, {op: 'chat', message: 'x'});
  await legacy.done;
  assert.equal(legacy.messages.at(-1).view.error, 'AI-сервис временно недоступен.');

  const cut = collect(setup({responses: [sseResponse([['delta', {text: 'обрыв'}]])]}).gateway, {op: 'chat', message: 'x'});
  await cut.done;
  assert.equal(cut.messages.at(-1).view.error, 'Соединение прервалось до конца ответа.');
  assert.equal(cut.messages.at(-1).view.text, 'обрыв');
});

test('stream: abort marks the answer as interrupted', async () => {
  const fetchImpl = (url, init) => new Promise((_resolve, reject) => {
    init.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), {name: 'AbortError'})));
  });
  const gateway = createGateway({baseUrl: base, auth: fakeAuth(), store: memoryStore(), client: {}, fetchImpl});
  const run = collect(gateway, {op: 'chat', message: 'x'});
  await new Promise(r => setImmediate(r));
  run.abort();
  await run.done;
  assert.equal(run.messages.at(-1).view.error, 'Соединение прервалось до конца ответа.');
});

test('stream brief: streamed, or "unsupported" without the endpoint', async () => {
  const brief = collect(setup({responses: [sseResponse([['delta', {text: 'Сводка'}], ['done', {}]])]}).gateway, {op: 'brief'});
  await brief.done;
  assert.equal(brief.messages.at(-1).view.text, 'Сводка');

  const none = collect(setup({responses: [textResponse('nope', 404)]}).gateway, {op: 'brief'});
  await none.done;
  assert.equal(none.messages.at(-1).op, 'unsupported');
});
