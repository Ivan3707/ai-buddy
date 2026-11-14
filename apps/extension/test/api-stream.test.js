import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decideStreamFallback, HttpError, streamChat } from '../lib/api.js';
import { applyStreamEvent, createAssistantState } from '../lib/stream-events.js';
import { fakeFetch, jsonResponse, sseResponse, textResponse } from './helpers.js';

const base = 'http://127.0.0.1:8080';

function collector() {
  let state = createAssistantState();
  return {
    onEvent: event => { state = applyStreamEvent(state, event); },
    get state() { return state; },
  };
}

test('decideStreamFallback', () => {
  assert.equal(decideStreamFallback({status: 404, contentType: 'text/plain; charset=utf-8'}), 'legacy');
  assert.equal(decideStreamFallback({status: 404, contentType: 'application/json', sentConversationId: true}), 'reset-conversation');
  assert.equal(decideStreamFallback({status: 404, contentType: 'application/problem+json', sentConversationId: true}), 'reset-conversation');
  assert.equal(decideStreamFallback({status: 404, contentType: 'application/json', sentConversationId: false}), 'legacy');
  assert.equal(decideStreamFallback({status: 404}), 'legacy');
  assert.equal(decideStreamFallback({status: 405}), 'legacy');
  assert.equal(decideStreamFallback({status: 501}), 'legacy');
  assert.equal(decideStreamFallback({status: 401}), 'unauthorized');
  assert.equal(decideStreamFallback({status: 502}), 'error');
  assert.equal(decideStreamFallback({status: 400}), 'error');
});

test('streams events from /api/chat/stream with auth and the request body', async () => {
  const fetchImpl = fakeFetch(sseResponse([
    ['meta', {conversation_id: 'c1', message_id: 'm1'}],
    ['delta', {text: 'Ответ ✓'}],
    ['done', {finish_reason: 'answered', mode: 'mock'}],
  ]));
  const out = collector();
  const result = await streamChat({
    baseUrl: base, token: 'jwt', fetchImpl, onEvent: out.onEvent,
    request: {message: 'Привет', conversation_id: 'c1'},
    legacyChat: () => assert.fail('legacy must not be called'),
  });
  assert.deepEqual(result, {transport: 'stream', conversationReset: false});
  assert.equal(out.state.text, 'Ответ ✓');
  const [call] = fetchImpl.calls;
  assert.equal(call.url, `${base}/api/chat/stream`);
  assert.equal(call.init.method, 'POST');
  assert.equal(call.init.headers.Authorization, 'Bearer jwt');
  assert.equal(call.init.headers.Accept, 'text/event-stream');
  assert.deepEqual(call.body, {message: 'Привет', conversation_id: 'c1'});
});

test('falls back to legacy POST /api/chat when the route is missing (Go 404 text/plain)', async () => {
  const fetchImpl = fakeFetch(textResponse('404 page not found\n', 404));
  const out = collector();
  const asked = [];
  const result = await streamChat({
    baseUrl: base, token: 'jwt', fetchImpl, onEvent: out.onEvent,
    request: {message: 'Как оформить отпуск?'},
    legacyChat: async message => {
      asked.push(message);
      return {answer: 'Через HR-портал', sources: [{id: 's', title: 'Отпуск', url: 'https://hr.example.com/leave'}], mode: 'mock'};
    },
  });
  assert.equal(result.transport, 'legacy');
  assert.deepEqual(asked, ['Как оформить отпуск?']);
  assert.equal(out.state.text, 'Через HR-портал');
  assert.equal(out.state.finished, true);
});

test('preferLegacy skips the stream endpoint entirely', async () => {
  const fetchImpl = fakeFetch();
  const out = collector();
  const result = await streamChat({
    baseUrl: base, token: 'jwt', fetchImpl, onEvent: out.onEvent, preferLegacy: true,
    request: {message: 'x'}, legacyChat: async () => ({answer: 'ok', sources: [], mode: 'mock'}),
  });
  assert.equal(result.transport, 'legacy');
  assert.equal(fetchImpl.calls.length, 0);
});

test('unknown conversation (JSON 404) retries once without conversation_id', async () => {
  const fetchImpl = fakeFetch(
    jsonResponse({error: 'conversation not found'}, 404),
    sseResponse([['meta', {conversation_id: 'c2', message_id: 'm2'}], ['done', {finish_reason: 'answered', mode: 'mock'}]]),
  );
  const out = collector();
  const result = await streamChat({
    baseUrl: base, token: 'jwt', fetchImpl, onEvent: out.onEvent,
    request: {message: 'x', conversation_id: 'old', page_context: {url: 'https://a.example'}},
    legacyChat: () => assert.fail('no legacy'),
  });
  assert.deepEqual(result, {transport: 'stream', conversationReset: true});
  assert.deepEqual(fetchImpl.calls[1].body, {message: 'x', page_context: {url: 'https://a.example'}});
  assert.equal(out.state.conversationId, 'c2');
});

test('401 notifies the panel and throws', async () => {
  let notified = 0;
  // Node has no window-level EventTarget; emulate the extension page.
  const target = new EventTarget();
  target.addEventListener('ai-buddy:unauthorized', () => { notified += 1; });
  globalThis.dispatchEvent = event => target.dispatchEvent(event);
  try {
    const fetchImpl = fakeFetch(jsonResponse({error: 'unauthorized'}, 401));
    await assert.rejects(
      streamChat({baseUrl: base, token: 'bad', fetchImpl, onEvent: () => {}, request: {message: 'x'}, legacyChat: () => {}}),
      error => error instanceof HttpError && error.status === 401 && error.message === 'unauthorized',
    );
    assert.equal(notified, 1);
  } finally {
    delete globalThis.dispatchEvent;
  }
});

test('other HTTP errors throw HttpError with a fallback message for non-JSON bodies', async () => {
  const fetchImpl = fakeFetch(textResponse('bad gateway', 502));
  await assert.rejects(
    streamChat({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}, request: {message: 'x'}, legacyChat: () => {}}),
    error => error instanceof HttpError && error.status === 502 && error.message === 'Backend: HTTP 502',
  );
});

test('a 200 response that is not an event stream is rejected', async () => {
  const fetchImpl = fakeFetch(jsonResponse({answer: 'x'}));
  await assert.rejects(
    streamChat({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}, request: {message: 'x'}, legacyChat: () => {}}),
    HttpError,
  );
});
