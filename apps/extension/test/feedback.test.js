import { test } from 'node:test';
import assert from 'node:assert/strict';
import { sendFeedback } from '../lib/api.js';
import { fakeFetch, jsonResponse, textResponse } from './helpers.js';

const base = 'http://127.0.0.1:8080';

test('sendFeedback posts the rating for the message', async () => {
  const fetchImpl = fakeFetch(new Response(null, {status: 204}));
  assert.deepEqual(await sendFeedback({baseUrl: base, token: 'jwt', fetchImpl}, 'm-1', 'up'), {saved: true});
  assert.equal(fetchImpl.calls[0].url, `${base}/api/chat/messages/m-1/feedback`);
  assert.deepEqual(fetchImpl.calls[0].body, {rating: 'up'});
  assert.equal(fetchImpl.calls[0].init.headers.Authorization, 'Bearer jwt');
});

test('sendFeedback ignores 404/405 (endpoint not deployed yet)', async () => {
  const fetchImpl = fakeFetch(textResponse('404 page not found', 404), jsonResponse({error: 'x'}, 405));
  assert.deepEqual(await sendFeedback({baseUrl: base, token: 't', fetchImpl}, 'm', 'down'), {saved: false});
  assert.deepEqual(await sendFeedback({baseUrl: base, token: 't', fetchImpl}, 'm', 'down'), {saved: false});
});

test('sendFeedback rethrows other errors and validates the rating', async () => {
  const fetchImpl = fakeFetch(jsonResponse({error: 'boom'}, 500));
  await assert.rejects(sendFeedback({baseUrl: base, token: 't', fetchImpl}, 'm', 'up'), error => error.status === 500);
  await assert.rejects(sendFeedback({baseUrl: base, token: 't', fetchImpl}, 'm', 'meh'), TypeError);
});
