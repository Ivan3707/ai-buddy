import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  clearToken,
  readToken,
  restoreSession,
  saveToken,
  tokenKey,
} from '../../../apps/extension/popup-session.js';

function fakeSessionStorage(initial = {}) {
  const data = {...initial};
  return {
    data,
    async get(key) { return {[key]: data[key]}; },
    async set(values) { Object.assign(data, values); },
    async remove(key) { delete data[key]; },
  };
}

test('extension session token survives popup reopen and logout clears it', async () => {
  const storage = fakeSessionStorage();
  await saveToken(storage, 'session-jwt');
  assert.equal(await readToken(storage), 'session-jwt');

  const reopened = await restoreSession(storage, async token => {
    assert.equal(token, 'session-jwt');
    return {email: 'buddy@example.com'};
  });
  assert.equal(reopened.status, 'authenticated');
  assert.equal(reopened.user.email, 'buddy@example.com');

  await clearToken(storage);
  assert.equal(await readToken(storage), '');
});

test('extension removes a rejected JWT but preserves it on network failure', async () => {
  const rejected = fakeSessionStorage({[tokenKey]: 'expired-jwt'});
  const unauthorized = await restoreSession(rejected, async () => {
    throw {status: 401};
  });
  assert.equal(unauthorized.status, 'unauthorized');
  assert.equal(await readToken(rejected), '');

  const offline = fakeSessionStorage({[tokenKey]: 'valid-jwt'});
  const unavailable = await restoreSession(offline, async () => {
    throw new TypeError('fetch failed');
  });
  assert.equal(unavailable.status, 'unavailable');
  assert.equal(await readToken(offline), 'valid-jwt');
});
