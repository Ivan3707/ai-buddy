// Mounts buddy/widget.js (the shared Бади widget) in jsdom with a scripted env: env.call answers from `handlers`,
// env.stream is captured so a test can push view models, env.storage is a Map.
import { mock } from 'node:test';
import { flush, installDom } from './dom.js';

export const page = installDom();
await import('../buddy/widget.js');

export const TODAY = new Date().toISOString().slice(0, 10);

export function view(over = {}) {
  return {status: '', text: '', sources: [], finished: false, error: null, card: null, form: null, messageId: null, mood: 'thinking', ...over};
}

export function submit(form) {
  form.dispatchEvent(new page.window.Event('submit', {bubbles: true, cancelable: true}));
}

export function key(target, k, init = {}) {
  const event = new page.window.KeyboardEvent('keydown', {key: k, bubbles: true, cancelable: true, composed: true, ...init});
  target.dispatchEvent(event);
  return event;
}

export function pointer(target, type, init = {}) {
  target.dispatchEvent(new page.window.MouseEvent(type, {bubbles: true, cancelable: true, composed: true, button: 0, ...init}));
}

/** Advances the mocked timers and lets the promise chains settle. */
export async function tick(ms) {
  mock.timers.tick(ms);
  await flush();
}

export async function mountWidget({handlers = {}, features, stored = {}, env: extra = {}, show = true} = {}) {
  const calls = [];
  const streams = [];
  const data = new Map(Object.entries({buddyGreeting: TODAY, ...stored}));
  const all = {
    session: {authenticated: true, user: {id: 'u1', name: 'Аня'}},
    requests: {changes: []},
    suggestions: {suggestions: []},
    hint: {hint: null},
    ...handlers,
  };
  const env = {
    async call(message) {
      calls.push(message);
      const answer = all[message.op];
      return typeof answer === 'function' ? answer(message) : answer;
    },
    stream(payload, onMessage) {
      const s = {payload, aborted: false, send: message => onMessage(message), view: v => onMessage({op: 'view', view: view(v)})};
      streams.push(s);
      return () => { s.aborted = true; };
    },
    storage: {get: async k => data.get(k), set: async (k, v) => { data.set(k, v); }},
    features,
    ...extra,
  };
  const api = globalThis.AIBuddyWidget.mount(env);
  const root = page.roots.at(-1);
  if (show) api.show();
  await flush();
  const $ = selector => root.querySelector(selector);
  const $$ = selector => [...root.querySelectorAll(selector)];
  return {
    api, root, env, calls, streams, data, $, $$, handlers: all,
    ops: () => calls.map(c => c.op),
    lastStream: () => streams.at(-1),
    async open() { await api.open(); await flush(); },
    async send(text) {
      $('.composer textarea').value = text;
      submit($('.composer'));
      await flush();
    },
  };
}
