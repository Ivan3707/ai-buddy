// Test DOM: a jsdom window whose globals are exposed on globalThis, so the classic scripts of buddy/ (widget.js,
// content.js, bridge.js) run in node as they do in a page. Each test file runs in its own process, so the globals
// do not leak between files.
import { JSDOM, VirtualConsole } from 'jsdom';

const GLOBALS = [
  'window', 'document', 'location', 'HTMLElement', 'HTMLInputElement', 'HTMLTextAreaElement', 'HTMLFormElement',
  'MutationObserver', 'FormData', 'Event', 'CustomEvent', 'KeyboardEvent', 'MouseEvent', 'MessageEvent', 'sessionStorage',
];
const FUNCTIONS = ['getComputedStyle', 'addEventListener', 'removeEventListener', 'dispatchEvent', 'postMessage'];

export function installDom({url = 'https://intranet.example.com/page', title = 'Портал', body = ''} = {}) {
  const dom = new JSDOM(`<!doctype html><html><head><title>${title}</title></head><body>${body}</body></html>`, {url, pretendToBeVisual: true, virtualConsole: new VirtualConsole()}); // silent: jsdom has no navigation
  const w = dom.window;
  // The widget renders into a closed shadow root: keep a handle on it for the assertions.
  const roots = [];
  const attachShadow = w.Element.prototype.attachShadow;
  w.Element.prototype.attachShadow = function (init) { const root = attachShadow.call(this, init); roots.push(root); return root; };
  // Not implemented by jsdom.
  w.Element.prototype.setPointerCapture = function () {};
  w.Element.prototype.scrollIntoView = function () { this.dataset.scrolledIntoView = '1'; };
  // jsdom has no named properties on forms (form.password, form.date_from...), browsers do.
  for (const name of ['type', 'email', 'password', 'kind', 'date_from', 'date_to', 'comment', 'system', 'resource', 'access_level',
    'justification', 'starts_at', 'ends_at', 'question', 'content', 'description']) {
    if (!(name in w.HTMLFormElement.prototype)) {
      Object.defineProperty(w.HTMLFormElement.prototype, name, {configurable: true, get() { return this.elements.namedItem(name) ?? undefined; }});
    }
  }
  Object.defineProperty(w.HTMLElement.prototype, 'innerText', {configurable: true, get() { return this.textContent; }});

  for (const name of GLOBALS) Object.defineProperty(globalThis, name, {configurable: true, writable: true, value: w[name]});
  for (const name of FUNCTIONS) globalThis[name] = w[name].bind(w);
  for (const name of ['innerWidth', 'innerHeight']) Object.defineProperty(globalThis, name, {configurable: true, get: () => w[name]});
  let selected = '';
  globalThis.getSelection = () => ({toString: () => selected});
  return {dom, window: w, roots, select: text => { selected = text; }};
}

export const flush = async (rounds = 8) => { for (let i = 0; i < rounds; i += 1) await new Promise(resolve => setImmediate(resolve)); };
