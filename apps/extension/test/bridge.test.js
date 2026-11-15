// buddy/bridge.js: the guarded link between a page and the background worker.
import assert from 'node:assert/strict';
import { mock, test } from 'node:test';

function event() {
  const listeners = [];
  return {listeners, addListener: fn => listeners.push(fn), emit: (...args) => listeners.forEach(fn => fn(...args))};
}

const ports = [];
const chromeMock = {
  runtime: {
    id: 'ext-id',
    sendMessage: async message => ({echo: message}),
    connect: ({name}) => {
      const port = {name, posted: [], onMessage: event(), onDisconnect: event(), postMessage(m) { if (port.broken) throw new Error('gone'); port.posted.push(m); }};
      ports.push(port);
      return port;
    },
  },
  storage: {local: {
    data: {buddyPos: {fx: 1}},
    get: async key => ({[key]: chromeMock.storage.local.data[key]}),
    set: async obj => { Object.assign(chromeMock.storage.local.data, obj); },
  }},
};
globalThis.chrome = chromeMock;
await import('../buddy/bridge.js');
const bridge = globalThis.AIBuddyBridge;

test('loads once and exposes the API', async () => {
  const same = bridge;
  await import('../buddy/bridge.js');
  assert.equal(globalThis.AIBuddyBridge, same);
  assert.equal(bridge.alive(), true);
});

test('call() forwards to the worker and turns a dead context into an error', async () => {
  assert.deepEqual(await bridge.call({op: 'session'}), {echo: {op: 'session'}});
  chromeMock.runtime.sendMessage = async () => { throw new Error('Extension context invalidated'); };
  assert.deepEqual(await bridge.call({op: 'session'}), {error: 'extension reloaded'});
  const id = chromeMock.runtime.id;
  delete chromeMock.runtime.id;
  assert.equal(bridge.alive(), false);
  assert.deepEqual(await bridge.call({op: 'session'}), {error: 'extension reloaded'});
  chromeMock.runtime.id = id;
});

test('storage reads and writes chrome.storage.local, quietly after a reload', async () => {
  assert.deepEqual(await bridge.storage.get('buddyPos'), {fx: 1});
  await bridge.storage.set('buddyMode', 'full');
  assert.equal(chromeMock.storage.local.data.buddyMode, 'full');

  const {get, set} = chromeMock.storage.local;
  chromeMock.storage.local.get = async () => { throw new Error('invalidated'); };
  chromeMock.storage.local.set = async () => { throw new Error('invalidated'); };
  assert.equal(await bridge.storage.get('buddyPos'), undefined);
  await bridge.storage.set('x', 1);
  chromeMock.storage.local.get = get; chromeMock.storage.local.set = set;

  const runtime = chromeMock.runtime;
  Object.defineProperty(chromeMock, 'runtime', {configurable: true, get() { throw new Error('invalidated'); }});
  assert.equal(bridge.alive(), false);
  assert.equal(await bridge.storage.get('buddyPos'), undefined);
  await bridge.storage.set('y', 2);
  assert.equal(chromeMock.storage.local.data.y, undefined);
  delete chromeMock.runtime;
  chromeMock.runtime = runtime;
});

test('stream() shares one port, routes answers by request id and aborts', async () => {
  const a = [], b = [];
  const abortA = bridge.stream({op: 'chat', message: 'one'}, m => a.push(m));
  bridge.stream({op: 'brief'}, m => b.push(m));
  assert.equal(ports.length, 1);
  const port = ports[0];
  assert.equal(port.name, 'buddy');
  assert.equal(port.posted.length, 2);
  const [first, second] = port.posted;
  assert.equal(first.message, 'one');
  assert.notEqual(first.requestId, second.requestId);
  port.onMessage.emit({requestId: second.requestId, op: 'view', view: {text: 'B'}});
  port.onMessage.emit({requestId: first.requestId, op: 'view', view: {text: 'A'}});
  port.onMessage.emit({requestId: 999, op: 'view'});
  assert.deepEqual(a.map(m => m.view.text), ['A']);
  assert.deepEqual(b.map(m => m.view.text), ['B']);
  abortA();
  assert.deepEqual(port.posted.at(-1), {op: 'abort'});
  port.broken = true;
  abortA(); // the port is gone: nothing thrown

  // a broken port reports "reloaded" once and the next stream connects again
  const c = [];
  const abortC = bridge.stream({op: 'chat', message: 'two'}, m => c.push(m));
  assert.equal(c.length, 1);
  assert.equal(c[0].view.error, 'Расширение обновилось. Обновите страницу.');
  assert.equal(c[0].view.finished, true);
  abortC();
  bridge.stream({op: 'chat', message: 'three'}, () => {});
  assert.equal(ports.length, 2);
  ports[1].onDisconnect.emit();
  bridge.stream({op: 'chat', message: 'four'}, () => {});
  assert.equal(ports.length, 3);

  const id = chromeMock.runtime.id;
  delete chromeMock.runtime.id;
  const d = [];
  bridge.stream({op: 'chat', message: 'x'}, m => d.push(m));
  assert.equal(d[0].view.mood, 'sad');
  chromeMock.runtime.id = id;
});

test('onInvalidated() runs the cleanup once the extension is gone', () => {
  mock.timers.enable({apis: ['setInterval']});
  let cleaned = 0;
  bridge.onInvalidated(() => { cleaned += 1; });
  mock.timers.tick(3000);
  assert.equal(cleaned, 0);
  delete chromeMock.runtime.id;
  mock.timers.tick(3000);
  mock.timers.tick(3000);
  assert.equal(cleaned, 1);
  mock.timers.reset();
});
