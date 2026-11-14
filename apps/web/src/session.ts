import { ApiError, getMe, linkSession, login, logoutSession, refreshSession, type User } from '@ai-buddy/client';
import { createAuth } from '@buddy/lib/auth.js';

// The site's session, persisted in localStorage so that it survives closing the tab and the browser.
// The logic (short access token, rotated refresh token, refresh under a Web Lock, logout) is shared with the extension:
// apps/extension/lib/auth.js. Hardening for production: keep the refresh token in an HttpOnly cookie instead.
const PREFIX = 'ai-buddy:';
const read = (key: string): unknown => { try { const raw = localStorage.getItem(PREFIX + key); return raw === null ? undefined : JSON.parse(raw); } catch { return undefined; } };

export const auth = createAuth({
  baseUrl: '',
  storage: {
    get: async key => read(key),
    set: async (key, value) => { try { localStorage.setItem(PREFIX + key, JSON.stringify(value)); } catch { /* private mode */ } },
    remove: async key => { try { localStorage.removeItem(PREFIX + key); } catch { /* ignore */ } },
  },
  client: { login, refreshSession, linkSession, logoutSession },
  // another tab (or the extension handshake in another tab) changed the session
  watch: onChange => {
    const handler = (event: StorageEvent) => { if (event.key === null || event.key === `${PREFIX}auth`) onChange(); };
    addEventListener('storage', handler);
    return () => removeEventListener('storage', handler);
  },
});

export const hasStoredSession = () => { try { return localStorage.getItem(`${PREFIX}auth`) !== null; } catch { return false; } };

/** The signed-in user and a valid access token, or null. A revoked session is cleared. */
export async function currentUser(): Promise<{ user: User; token: string } | null> {
  const token = await auth.token();
  if (!token) return null;
  try {
    return { user: await getMe('', token), token };
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) throw error;
    const next = await auth.token({ force: true, stale: token });
    if (next && next !== token) return { user: await getMe('', next), token: next };
    await auth.clear();
    return null;
  }
}
