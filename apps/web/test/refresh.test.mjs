import assert from 'node:assert/strict';
import { test } from 'node:test';
import { singleFlightRefresh } from '../src/refresh.js';

test('concurrent refresh calls share a single request', async () => {
  let calls = 0;
  let resolve;
  const refresh = singleFlightRefresh(() => {
    calls++;
    if (calls > 1) return Promise.resolve('new');
    return new Promise(done => { resolve = done; });
  });
  const first = refresh('old');
  const second = refresh('old');
  assert.equal(first, second);
  assert.equal(calls, 1);
  resolve('new');
  assert.equal(await first, 'new');
  assert.equal(await refresh('new'), 'new');
  assert.equal(calls, 2);
});

test('failed refresh is shared and a later call can retry', async () => {
  let calls = 0;
  const refresh = singleFlightRefresh(async () => {
    if (++calls === 1) throw new Error('unavailable');
    return 'new';
  });
  const first = refresh('old');
  const second = refresh('old');
  await assert.rejects(first, /unavailable/);
  await assert.rejects(second, /unavailable/);
  assert.equal(calls, 1);
  assert.equal(await refresh('old'), 'new');
});
