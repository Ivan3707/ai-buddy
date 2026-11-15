// Loads buddy/content.js with a fake widget (globalThis.AIBuddyWidget), a fake bridge and a fake chrome.*,
// on a page of the given url. One url per test file: the script installs itself once per page.
import { flush, installDom } from './dom.js';

function event() {
  const listeners = [];
  return {listeners, addListener: fn => listeners.push(fn), emit: (...args) => listeners.forEach(fn => fn(...args))};
}

export async function loadContent({url, stored = {}, callAnswer = {ok: true}} = {}) {
  const page = installDom({url});
  const posted = [];
  page.window.postMessage = (message, origin) => { posted.push({message, origin}); };
  const calls = [];
  const buddy = {
    shown: false,
    log: [],
    show() { buddy.shown = true; buddy.log.push('show'); },
    hide() { buddy.shown = false; buddy.log.push('hide'); },
    remove() { buddy.log.push('remove'); },
    isShown: () => buddy.shown,
    open: async () => { buddy.log.push('open'); },
    toggle() { buddy.log.push('toggle'); },
    ask: async q => { buddy.log.push(['ask', q]); },
    say: async (text, options) => { buddy.log.push(['say', text, options]); },
    refresh: async () => { buddy.log.push('refresh'); },
    syncPosition: async () => { buddy.log.push('sync'); },
  };
  let mountEnv = null, invalidated = null;
  const storage = {data: {...stored}, get: async key => storage.data[key], set: async (key, value) => { storage.data[key] = value; }};
  globalThis.AIBuddyWidget = {mount: env => { mountEnv = env; return buddy; }};
  globalThis.AIBuddyBridge = {
    call: async message => { calls.push(message); return callAnswer; },
    stream: () => () => {},
    storage,
    onInvalidated: fn => { invalidated = fn; },
  };
  const chrome = {storage: {onChanged: event()}, runtime: {onMessage: event()}};
  globalThis.chrome = chrome;
  let announced = 0;
  page.window.addEventListener('ai-buddy:extension', () => { announced += 1; });
  await import('../buddy/content.js');
  await flush();
  const fromSite = (data, {origin = page.window.location.origin, source = page.window} = {}) => {
    page.window.dispatchEvent(new page.window.MessageEvent('message', {data, origin, source}));
  };
  return {page, posted, calls, buddy, storage, chrome, env: () => mountEnv, invalidate: () => invalidated(), announced: () => announced, fromSite};
}
