// buddy/content.js on an ordinary site: mounts the widget, follows the shared shown/hidden state, the toolbar icon.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { flush } from './dom.js';
import { loadContent } from './content-harness.js';

const ctx = await loadContent({url: 'https://grafana.example.com/d/1'});

test('mounts the widget with the extension features and shows it when not hidden', () => {
  const env = ctx.env();
  assert.deepEqual(env.features, {login: true, logout: true, pageContext: true, hide: true, requests: true});
  assert.equal(env.call, globalThis.AIBuddyBridge.call);
  assert.equal(env.portalUrl('assistant', 'r 1'), 'http://127.0.0.1:5173/portal/requests/assistant/r%201');
  assert.deepEqual(ctx.buddy.log, ['show']);
  assert.equal(document.documentElement.dataset.aiBuddyExtension, undefined, 'the site bridge is only on the web app');
  assert.equal(ctx.announced(), 0);
});

test('× hides Бади in every window; storage changes hide, show and move him', async () => {
  await ctx.env().onHide();
  assert.equal(ctx.storage.data.buddyVisible, false);
  const changed = ctx.chrome.storage.onChanged;
  ctx.buddy.log.length = 0;
  changed.emit({buddyVisible: {newValue: false}}, 'session');
  assert.deepEqual(ctx.buddy.log, []);
  changed.emit({buddyVisible: {newValue: false}}, 'local');
  changed.emit({buddyVisible: {newValue: true}}, 'local');
  changed.emit({buddyPos: {newValue: {fx: 0}}}, 'local');
  changed.emit({other: {}}, 'local');
  assert.deepEqual(ctx.buddy.log, ['hide', 'show', 'sync']);
});

test('toolbar icon: shows a hidden Бади and opens the chat, or toggles it', () => {
  const onMessage = ctx.chrome.runtime.onMessage;
  ctx.buddy.shown = false;
  ctx.buddy.log.length = 0;
  onMessage.emit({op: 'toggle', wasShown: false});
  assert.deepEqual(ctx.buddy.log, ['show', 'open']);
  ctx.buddy.log.length = 0;
  onMessage.emit({op: 'toggle', wasShown: true});
  assert.deepEqual(ctx.buddy.log, ['toggle']);
  ctx.buddy.log.length = 0;
  onMessage.emit({op: 'sso-changed'});
  onMessage.emit(undefined);
  assert.deepEqual(ctx.buddy.log, []);
  assert.equal(ctx.posted.length, 0);
});

test('page messages are not heard outside the web app; an orphaned script removes Бади', async () => {
  ctx.fromSite({source: 'ai-buddy-web', op: 'ask', question: 'x'});
  await flush();
  assert.ok(!ctx.buddy.log.some(e => Array.isArray(e) && e[0] === 'ask'));
  ctx.invalidate();
  assert.equal(ctx.buddy.log.at(-1), 'remove');
});
