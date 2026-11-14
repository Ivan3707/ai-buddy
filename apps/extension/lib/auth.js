// Session core shared by the extension's background worker and the web app (same code, injected storage).
//
// How it works (the usual production scheme):
// - access token: a short-lived JWT; refreshed shortly before it expires;
// - refresh token: long-lived, opaque, ROTATED on every use and stored hashed on the server; a replayed one revokes the
//   session. So only one context may refresh at a time: the refresh runs under a Web Lock (all tabs of the site, the
//   background worker) and re-reads the storage inside the lock, so a second tab reuses what the first one obtained;
// - the session is persisted (localStorage on the site, chrome.storage.local in the extension): it survives closing
//   the tab, the browser and restarts, until logout or until the refresh token expires (30 days, sliding);
// - every client (the site, the extension) has its OWN session of the same user: one shared refresh chain would be
//   revoked by replay detection as soon as two clients refresh. A client opens a session for the other one with
//   POST /api/auth/link (see link()/adopt());
// - logout revokes the session on the server and leaves a "signed out at" mark that the other client uses to follow.
const SKEW_MS = 60_000;
const KEY = 'auth';
const SIGNED_OUT = 'signedOutAt';

const isString = value => typeof value === 'string' && value.length > 0;

export function subjectOf(token) {
  try {
    const payload = String(token).split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    return String(JSON.parse(atob(payload)).sub || '');
  } catch { return ''; }
}

/**
 * @param {object} o
 * @param {string} o.baseUrl
 * @param {{get(key: string): Promise<any>, set(key: string, value: any): Promise<void>, remove(key: string): Promise<void>}} o.storage
 * @param {{login: Function, refreshSession: Function, linkSession: Function, logoutSession: Function}} o.client
 * @param {(name: string, fn: () => Promise<any>) => Promise<any>} [o.lock]
 * @param {(onChange: () => void) => (() => void)} [o.watch] subscribes to changes made by other contexts
 * @param {() => number} [o.now]
 */
export function createAuth({baseUrl, storage, client, lock, watch, now = Date.now}) {
  const withLock = lock || ((name, fn) => (globalThis.navigator?.locks ? globalThis.navigator.locks.request(name, fn) : fn()));

  async function read() {
    const s = await storage.get(KEY);
    return s && isString(s.access) && isString(s.refresh) && Number.isFinite(s.expiresAt) ? s : null;
  }
  const write = s => storage.set(KEY, s);

  async function info() {
    const s = await read();
    const out = await storage.get(SIGNED_OUT);
    return {sub: s?.sub || '', loginAt: s?.loginAt || 0, signedOutAt: Number.isFinite(out) ? out : 0};
  }

  function fromPair(pair, previous, extra = {}) {
    return {
      access: pair.access_token, refresh: pair.refresh_token,
      expiresAt: now() + (Number(pair.expires_in) > 0 ? Number(pair.expires_in) : 900) * 1000,
      sub: subjectOf(pair.access_token), loginAt: previous?.loginAt || now(), ...extra,
    };
  }

  /** A valid access token ('' when signed out). Refreshes it under the lock when it is about to expire. */
  async function token({force = false, stale = ''} = {}) {
    let s = await read();
    if (!s) return '';
    const fresh = value => value.expiresAt - now() > SKEW_MS;
    if (!force && fresh(s)) return s.access;
    return withLock('ai-buddy-auth', async () => {
      s = await read(); // another tab / the worker may have refreshed while we waited for the lock
      if (!s) return '';
      if (fresh(s) && (!force || s.access !== stale)) return s.access;
      try {
        const pair = await client.refreshSession(baseUrl, s.refresh);
        const next = fromPair(pair, s);
        await write(next);
        return next.access;
      } catch (error) {
        if (error?.status === 401) { await storage.remove(KEY); return ''; } // expired, revoked or replayed: signed out
        return s.access; // network trouble: keep the session, the caller will see the failure itself
      }
    });
  }

  /** Stores a new session. The previous one (other user, or an older session of this client) is revoked, best effort. */
  async function adopt(pair, {loginAt = now()} = {}) {
    const previous = await read();
    await write(fromPair(pair, undefined, {loginAt}));
    if (previous && previous.refresh !== pair.refresh_token) {
      void client.logoutSession(baseUrl, previous.access, previous.refresh).catch(() => {});
    }
  }

  /** Password sign-in. Returns the backend's login response (with the user). */
  async function login(email, password) {
    const result = await client.login(baseUrl, email, password);
    await adopt(result, {loginAt: now()});
    return result;
  }

  /** A NEW session of the same user for the other client (token exchange); our own session is untouched. */
  async function link() {
    return withLock('ai-buddy-auth', async () => {
      const s = await read();
      if (!s) return null;
      try {
        return await client.linkSession(baseUrl, s.refresh);
      } catch (error) {
        if (error?.status === 401) await storage.remove(KEY);
        return null;
      }
    });
  }

  /** Sign out: revoke on the server (best effort), forget locally, leave the mark the other client follows. */
  async function logout() {
    const s = await read();
    if (s) {
      const access = await token().catch(() => s.access);
      try { await client.logoutSession(baseUrl, access || s.access, s.refresh); } catch { /* the server may be down: still sign out here */ }
    }
    await storage.remove(KEY);
    await storage.set(SIGNED_OUT, now());
  }

  async function clear() { await storage.remove(KEY); }

  return {read, info, token, adopt, login, link, logout, clear, onChange: cb => (watch ? watch(cb) : () => {})};
}
