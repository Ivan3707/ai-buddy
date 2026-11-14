import { auth } from './session';

// Single sign-on with the Chrome extension (when installed): the site and the extension keep separate sessions of the
// same user; a handshake decides who adopts whose session (apps/extension/lib/sso.js). Talks to the extension's content
// script through window.postMessage: only plain data, only this origin.
type Pair = { access_token: string; refresh_token: string; expires_in: number };
type Reply = { action?: 'none' | 'push' | 'pull' | 'logout'; pair?: Pair; loginAt?: number; ok?: boolean; error?: string };

let seq = 0;
export const extensionPresent = () => document.documentElement.dataset.aiBuddyExtension === '1';

function ask(body: Record<string, unknown>, timeoutMs = 4000): Promise<Reply | null> {
  return new Promise(resolve => {
    const id = `sso-${++seq}`;
    const done = (value: Reply | null) => { clearTimeout(timer); removeEventListener('message', onMessage); resolve(value); };
    const timer = setTimeout(() => done(null), timeoutMs);
    const onMessage = (event: MessageEvent) => {
      if (event.source !== window || event.origin !== location.origin) return;
      const m = event.data as { source?: string; op?: string; id?: string } & Reply;
      if (m?.source === 'ai-buddy-extension' && m.op === 'sso-reply' && m.id === id) done(m);
    };
    addEventListener('message', onMessage);
    window.postMessage({ source: 'ai-buddy-web', op: 'sso', id, ...body }, location.origin);
  });
}

/** Compare sessions with the extension and adopt / hand over / sign out as decided. */
export async function handshake(hooks: { onSessionChanged(): Promise<void>; onSignedOut(): Promise<void> }) {
  if (!extensionPresent()) return;
  const run = async () => {
    const state = await auth.info();
    const reply = await ask({ request: 'handshake', state });
    if (!reply?.action) return;
    if (reply.action === 'push') {
      const pair = await auth.link();
      if (pair) await ask({ request: 'adopt', pair, loginAt: state.loginAt });
    } else if (reply.action === 'pull' && reply.pair) {
      await auth.adopt(reply.pair, { loginAt: reply.loginAt || Date.now() });
      await hooks.onSessionChanged();
    } else if (reply.action === 'logout') {
      await auth.logout();
      await hooks.onSignedOut();
    }
  };
  // one handshake at a time across the tabs of the site
  if (navigator.locks) await navigator.locks.request('ai-buddy-sso', run); else await run();
}

/** The user signed out on the site: the extension signs out as well. */
export const notifySignedOut = async () => { if (extensionPresent()) await ask({ request: 'logout' }); };
