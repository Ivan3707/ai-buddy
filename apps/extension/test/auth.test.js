import assert from 'node:assert/strict';
import test from 'node:test';
import { createAuth, subjectOf } from '../lib/auth.js';
import { decideSync } from '../lib/sso.js';

const b64 = value => Buffer.from(JSON.stringify(value)).toString('base64url');
const jwt = sub => `${b64({alg: 'HS256'})}.${b64({sub})}.sig`;

function setup({now = {t: 1_000_000}} = {}) {
  const data = new Map();
  const calls = [];
  let refreshImpl = async (_url, refresh) => ({access_token: jwt('u1'), refresh_token: `${refresh}+`, expires_in: 900});
  const client = {
    login: async () => ({access_token: jwt('u1'), refresh_token: 'r1', expires_in: 900, user: {id: 'u1'}}),
    refreshSession: async (...args) => { calls.push(['refresh', ...args]); return refreshImpl(...args); },
    linkSession: async (_url, refresh) => { calls.push(['link', refresh]); return {access_token: jwt('u1'), refresh_token: 'linked', expires_in: 900}; },
    logoutSession: async (_url, _access, refresh) => { calls.push(['logout', refresh]); },
  };
  const storage = {
    get: async key => data.get(key),
    set: async (key, value) => { data.set(key, value); },
    remove: async key => { data.delete(key); },
  };
  const auth = createAuth({baseUrl: '', storage, client, now: () => now.t, lock: (_name, fn) => fn()});
  return {auth, data, calls, now, setRefresh: fn => { refreshImpl = fn; }};
}

test('subjectOf reads the user id from the access token', () => {
  assert.equal(subjectOf(jwt('abc')), 'abc');
  assert.equal(subjectOf('garbage'), '');
});

test('login stores the session; a fresh token is returned without refreshing', async () => {
  const {auth, calls} = setup();
  await auth.login('a@b', 'x');
  assert.equal((await auth.info()).sub, 'u1');
  assert.equal(await auth.token(), jwt('u1'));
  assert.equal(calls.filter(c => c[0] === 'refresh').length, 0);
});

test('a token about to expire is refreshed and the refresh token rotates', async () => {
  const {auth, calls, now} = setup();
  await auth.login('a@b', 'x');
  now.t += 850_000; // 50 s left: inside the 60 s margin
  await auth.token();
  assert.deepEqual(calls.filter(c => c[0] === 'refresh'), [['refresh', '', 'r1']]);
  assert.equal((await auth.read()).refresh, 'r1+');
});

test('a rejected refresh token signs out; a network error keeps the session', async () => {
  const {auth, now, setRefresh, data} = setup();
  await auth.login('a@b', 'x');
  now.t += 850_000;
  setRefresh(async () => { throw Object.assign(new Error('offline'), {status: undefined}); });
  assert.equal(await auth.token(), jwt('u1'));
  assert.ok(data.has('auth'));
  setRefresh(async () => { throw Object.assign(new Error('revoked'), {status: 401}); });
  assert.equal(await auth.token(), '');
  assert.equal(data.has('auth'), false);
});

test('forced refresh reuses a token another tab already rotated', async () => {
  const {auth, calls, data} = setup();
  await auth.login('a@b', 'x');
  const stale = (await auth.read()).access;
  data.set('auth', {...(await auth.read()), access: jwt('u1') + 'x', expiresAt: 5_000_000}); // another tab refreshed
  assert.equal(await auth.token({force: true, stale}), jwt('u1') + 'x');
  assert.equal(calls.filter(c => c[0] === 'refresh').length, 0);
});

test('link opens another session without touching ours; adopt revokes the replaced one', async () => {
  const {auth, calls} = setup();
  await auth.login('a@b', 'x');
  const pair = await auth.link();
  assert.equal(pair.refresh_token, 'linked');
  assert.equal((await auth.read()).refresh, 'r1');
  await auth.adopt({access_token: jwt('u2'), refresh_token: 'r2', expires_in: 900}, {loginAt: 5});
  assert.deepEqual(calls.filter(c => c[0] === 'logout'), [['logout', 'r1']]);
  assert.equal((await auth.info()).sub, 'u2');
  assert.equal((await auth.info()).loginAt, 5);
});

test('logout revokes on the server and leaves a signed-out mark', async () => {
  const {auth, calls, now} = setup();
  await auth.login('a@b', 'x');
  now.t += 10;
  await auth.logout();
  assert.deepEqual(calls.filter(c => c[0] === 'logout'), [['logout', 'r1']]);
  const info = await auth.info();
  assert.equal(info.sub, '');
  assert.equal(info.signedOutAt, now.t);
});

test('single sign-on: who adopts whose session', () => {
  const none = {sub: '', loginAt: 0, signedOutAt: 0};
  const site = {sub: 'u1', loginAt: 100, signedOutAt: 0};
  const ext = {sub: 'u1', loginAt: 50, signedOutAt: 0};
  assert.equal(decideSync(none, none), 'none');
  assert.equal(decideSync(site, none), 'push');
  assert.equal(decideSync(none, ext), 'pull');
  assert.equal(decideSync(site, ext), 'none', 'the same user on both sides keeps both sessions');
  assert.equal(decideSync(site, {...ext, sub: 'u2'}), 'push', 'the newer sign-in wins');
  assert.equal(decideSync({...site, loginAt: 10}, {...ext, sub: 'u2'}), 'pull');
  assert.equal(decideSync(site, {...none, signedOutAt: 200}), 'logout', 'the extension signed out after the site signed in');
  assert.equal(decideSync(site, {...none, signedOutAt: 20}), 'push', 'the extension signed out long ago');
  assert.equal(decideSync({...none, signedOutAt: 300}, ext), 'ext-logout', 'the site signed out after the extension signed in');
});
