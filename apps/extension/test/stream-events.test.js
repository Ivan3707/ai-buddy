import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  applyStreamEvent,
  createAssistantState,
  errorFrom,
  failureCode,
  finalizeStream,
  legacyResponseToEvents,
  normalizeAction,
  normalizeSources,
  parseEventData,
  STATUS_LABELS,
  statusLabel,
} from '../lib/stream-events.js';

const ev = (type, data) => ({type, data: typeof data === 'string' ? data : JSON.stringify(data)});
const run = events => events.reduce(applyStreamEvent, createAssistantState());

test('full happy-path stream builds the assistant message', () => {
  const state = run([
    ev('meta', {conversation_id: 'c1', message_id: 'm1'}),
    ev('status', {stage: 'retrieving'}),
    ev('status', {stage: 'generating'}),
    ev('delta', {text: 'Привет'}),
    ev('delta', {text: ', мир'}),
    ev('sources', {sources: [{id: 's1', title: 'Отпуск', url: 'https://kb.example/leave', snippet: 'цитата'}]}),
    ev('done', {finish_reason: 'answered', mode: 'llm', latency_ms: 420}),
  ]);
  assert.equal(state.conversationId, 'c1');
  assert.equal(state.messageId, 'm1');
  assert.equal(state.text, 'Привет, мир');
  assert.deepEqual(state.sources, [{id: 's1', title: 'Отпуск', url: 'https://kb.example/leave', snippet: 'цитата'}]);
  assert.equal(state.finished, true);
  assert.equal(state.finishReason, 'answered');
  assert.equal(state.mode, 'llm');
  assert.equal(state.latencyMs, 420);
  assert.equal(state.stage, null);
  assert.equal(state.error, null);
});

test('status labels follow the stage and disappear when finished', () => {
  let state = createAssistantState();
  assert.equal(statusLabel(state), STATUS_LABELS.retrieving);
  state = applyStreamEvent(state, ev('status', {stage: 'preparing_action'}));
  assert.equal(statusLabel(state), 'Готовлю заявку…');
  state = applyStreamEvent(state, ev('status', {stage: 'unknown'}));
  assert.equal(state.stage, 'preparing_action');
  state = applyStreamEvent(state, ev('done', {}));
  assert.equal(statusLabel(state), '');
  assert.equal(state.finishReason, 'answered');
  assert.equal(state.mode, null);
  assert.equal(state.latencyMs, null);
});

test('action event stores a normalized pending action', () => {
  const state = run([
    ev('action', {action: {id: 'a1', type: 'leave_request', summary: 'Отгул 20.11', status: 'pending', params: {kind: 'day_off'}, created_at: 'x'}}),
    ev('done', {finish_reason: 'action_proposed', mode: 'mock'}),
  ]);
  assert.deepEqual(state.action, {id: 'a1', type: 'leave_request', summary: 'Отгул 20.11', status: 'pending', params: {kind: 'day_off'}});
  assert.equal(state.finishReason, 'action_proposed');
});

test('malformed action, sources and data are ignored', () => {
  const state = run([
    ev('action', {action: {id: 1}}),
    ev('sources', {sources: 'nope'}),
    ev('delta', {text: 5}),
    ev('delta', 'not json'),
    ev('delta', '[1,2]'),
    ev('meta', {conversation_id: 5}),
    ev('mystery', {x: 1}),
  ]);
  assert.deepEqual(state, createAssistantState());
});

test('error event finishes the stream; later events are ignored', () => {
  const state = run([
    ev('delta', {text: 'частично'}),
    ev('error', {code: 'rate_limited', message: 'slow down'}),
    ev('delta', {text: 'лишнее'}),
    ev('done', {}),
  ]);
  assert.equal(state.text, 'частично');
  assert.deepEqual(state.error, {code: 'rate_limited', message: 'Слишком много запросов. Подождите немного.'});
  assert.equal(state.finishReason, null);
});

test('errorFrom maps known codes and keeps the server message for unknown ones', () => {
  assert.equal(errorFrom('timeout').message, 'AI не ответил вовремя.');
  assert.deepEqual(errorFrom('weird', 'Сервер занят'), {code: 'internal', message: 'Сервер занят'});
  assert.deepEqual(errorFrom(undefined, ''), {code: 'internal', message: 'Внутренняя ошибка сервиса.'});
});

test('finalizeStream marks an interrupted stream but keeps finished ones', () => {
  const partial = run([ev('delta', {text: 'обрыв'})]);
  const finalized = finalizeStream(partial);
  assert.equal(finalized.error.code, 'interrupted');
  assert.equal(finalized.text, 'обрыв');
  assert.equal(finalizeStream(finalized, 'timeout'), finalized);
  assert.equal(finalizeStream(createAssistantState(), 'timeout').error.code, 'timeout');
});

test('failureCode maps thrown errors', () => {
  assert.equal(failureCode({name: 'StreamIdleTimeoutError'}), 'timeout');
  assert.equal(failureCode({name: 'TimeoutError'}), 'timeout');
  assert.equal(failureCode({status: 429}), 'rate_limited');
  assert.equal(failureCode({status: 502}), 'llm_unavailable');
  assert.equal(failureCode({status: 400}), 'internal');
  assert.equal(failureCode(new TypeError('Failed to fetch')), 'network');
  assert.equal(failureCode(undefined), 'network');
});

test('normalizers and parseEventData', () => {
  assert.equal(parseEventData('null'), null);
  assert.deepEqual(parseEventData('{"a":1}'), {a: 1});
  assert.deepEqual(normalizeSources([{id: 'a', title: 'b', url: 'c', snippet: ''}, {id: 'x'}]), [{id: 'a', title: 'b', url: 'c'}]);
  assert.deepEqual(normalizeSources(undefined), []);
  assert.deepEqual(normalizeSources([{id: 'a', title: 'b', url: 'c', refs: [2, 4]}, {id: 'd', title: 'e', url: 'f', refs: ['x']}]), [{id: 'a', title: 'b', url: 'c', refs: [2, 4]}, {id: 'd', title: 'e', url: 'f'}]);
  assert.deepEqual(normalizeAction({id: 'a', type: 't', summary: 's', error: 'boom'}), {id: 'a', type: 't', summary: 's', status: 'pending', params: {}, error: 'boom'});
  assert.equal(normalizeAction(null), null);
});

test('legacy response is converted to the same events', () => {
  const state = run(legacyResponseToEvents({
    answer: 'Ответ',
    sources: [{id: 's', title: 'Док', url: 'https://kb.example/doc'}],
    mode: 'mock',
  }));
  assert.equal(state.text, 'Ответ');
  assert.equal(state.sources.length, 1);
  assert.equal(state.finished, true);
  assert.equal(state.mode, 'mock');
  assert.equal(state.messageId, null);

  const empty = run(legacyResponseToEvents({}));
  assert.equal(empty.text, '');
  assert.equal(empty.mode, 'mock');
  assert.deepEqual(empty.sources, []);
});
