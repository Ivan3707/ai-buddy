import { test } from 'node:test';
import assert from 'node:assert/strict';
import { HttpError, streamBrief } from '../lib/api.js';
import { briefStorageKey, shouldRequestBrief, todayKey } from '../lib/brief.js';
import { applyStreamEvent, createAssistantState } from '../lib/stream-events.js';
import { fakeFetch, jsonResponse, sseResponse, textResponse } from './helpers.js';

const base = 'http://127.0.0.1:8080';

test('todayKey uses the local calendar date', () => {
  assert.equal(todayKey(new Date(2026, 10, 3, 23, 59)), '2026-11-03');
  assert.match(todayKey(), /^\d{4}-\d{2}-\d{2}$/);
});

test('shouldRequestBrief: once per user per day and only for a fresh dialog', () => {
  const today = '2026-11-13';
  assert.equal(shouldRequestBrief({userId: 'u1', lastBriefDate: undefined, today, hasConversation: false}), true);
  assert.equal(shouldRequestBrief({userId: 'u1', lastBriefDate: '2026-11-12', today, hasConversation: false}), true);
  assert.equal(shouldRequestBrief({userId: 'u1', lastBriefDate: today, today, hasConversation: false}), false);
  assert.equal(shouldRequestBrief({userId: 'u1', lastBriefDate: undefined, today, hasConversation: true}), false);
  assert.equal(shouldRequestBrief({userId: '', lastBriefDate: undefined, today, hasConversation: false}), false);
  assert.equal(briefStorageKey('u1'), 'briefDate:u1');
});

test('streamBrief streams events without a request body', async () => {
  const fetchImpl = fakeFetch(sseResponse([
    ['meta', {conversation_id: 'c', message_id: 'm'}],
    ['delta', {text: 'Доброе утро! Сегодня — доступ в Grafana.'}],
    ['done', {finish_reason: 'answered', mode: 'mock'}],
  ]));
  let state = createAssistantState();
  const result = await streamBrief({baseUrl: base, token: 'jwt', fetchImpl, onEvent: event => { state = applyStreamEvent(state, event); }});
  assert.deepEqual(result, {transport: 'stream'});
  assert.equal(state.text, 'Доброе утро! Сегодня — доступ в Grafana.');
  assert.equal(fetchImpl.calls[0].url, `${base}/api/chat/brief/stream`);
  assert.equal(fetchImpl.calls[0].init.body, undefined);
  assert.equal(fetchImpl.calls[0].init.headers['Content-Type'], undefined);
});

test('streamBrief is silently unsupported on 404/405', async () => {
  const fetchImpl = fakeFetch(textResponse('404 page not found', 404), textResponse('', 405));
  assert.deepEqual(await streamBrief({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}}), {transport: 'unsupported'});
  assert.deepEqual(await streamBrief({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}}), {transport: 'unsupported'});
});

test('streamBrief throws on other errors and notifies on 401', async () => {
  let notified = 0;
  globalThis.dispatchEvent = () => { notified += 1; };
  try {
    const fetchImpl = fakeFetch(jsonResponse({error: 'unauthorized'}, 401), jsonResponse({error: 'down'}, 502));
    await assert.rejects(streamBrief({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}}), error => error.status === 401);
    await assert.rejects(streamBrief({baseUrl: base, token: 't', fetchImpl, onEvent: () => {}}), error => error instanceof HttpError && error.message === 'down');
    assert.equal(notified, 1);
  } finally {
    delete globalThis.dispatchEvent;
  }
});
