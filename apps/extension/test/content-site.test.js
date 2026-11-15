// buddy/content.js on the AI-buddy web app: cannot be hidden, bridges single sign-on and the site's nudges.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { flush } from './dom.js';
import { loadContent } from './content-harness.js';

const ctx = await loadContent({url: 'http://127.0.0.1:5173/dashboard', stored: {buddyVisible: false}, callAnswer: {action: 'pull'}});
const origin = 'http://127.0.0.1:5173';

test('on the site Бади cannot be hidden and announces the extension', () => {
  assert.equal(ctx.env().features.hide, false);
  assert.deepEqual(ctx.buddy.log, ['show'], 'shown although the hidden flag is set');
  assert.equal(document.documentElement.dataset.aiBuddyExtension, '1');
  assert.equal(ctx.announced(), 1);
  ctx.buddy.log.length = 0;
  ctx.chrome.storage.onChanged.emit({buddyVisible: {newValue: false}}, 'local');
  assert.deepEqual(ctx.buddy.log, ['show']);
});

test('the extension signing in/out tells the site', () => {
  ctx.chrome.runtime.onMessage.emit({op: 'sso-changed'});
  assert.deepEqual(ctx.posted.at(-1), {message: {source: 'ai-buddy-extension', op: 'changed'}, origin});
});

test('single sign-on requests go to the worker and the reply back to the site', async () => {
  ctx.posted.length = 0;
  ctx.buddy.log.length = 0;
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'handshake', id: 'h1', state: {sub: 'u1', loginAt: 5}});
  await flush();
  assert.deepEqual(ctx.calls.at(-1), {op: 'sso-handshake', state: {sub: 'u1', loginAt: 5}, pair: undefined, loginAt: undefined});
  assert.deepEqual(ctx.posted.at(-1), {message: {source: 'ai-buddy-extension', op: 'sso-reply', id: 'h1', action: 'pull'}, origin});
  assert.deepEqual(ctx.buddy.log, ['refresh']);

  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'adopt', id: 'h2', pair: {access_token: 'a'}, loginAt: 7});
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'logout', id: 'h3'});
  await flush();
  assert.deepEqual(ctx.calls.slice(-2).map(c => c.op), ['sso-adopt', 'sso-logout']);
  assert.equal(ctx.calls.at(-2).loginAt, 7);

  const count = ctx.calls.length;
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'steal', id: 'x'});
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'handshake', id: 5});
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'handshake', id: 'x'}, {origin: 'https://evil.example'});
  ctx.fromSite({source: 'ai-buddy-web', op: 'sso', request: 'handshake', id: 'x'}, {source: null});
  ctx.fromSite({source: 'someone', op: 'sso', request: 'handshake', id: 'x'});
  ctx.fromSite(null);
  await flush();
  assert.equal(ctx.calls.length, count, 'only well-formed messages of the site window itself are heard');
});

test('the site asks a question or passes a nudge with buttons', async () => {
  ctx.buddy.shown = true;
  ctx.buddy.log.length = 0;
  ctx.fromSite({source: 'ai-buddy-web', op: 'ask', question: 'Что дальше?'});
  ctx.fromSite({source: 'ai-buddy-web', op: 'ask', question: ''});
  ctx.fromSite({source: 'ai-buddy-web', op: 'say', text: ''});
  ctx.fromSite({source: 'ai-buddy-web', op: 'other', text: 'x'});
  assert.deepEqual(ctx.buddy.log, [['ask', 'Что дальше?']]);

  ctx.buddy.log.length = 0;
  ctx.posted.length = 0;
  ctx.fromSite({source: 'ai-buddy-web', op: 'say', text: 'Задача идёт дольше плана', detail: 'на 2 дня', question: 'Как ускориться?', id: 'n1', mood: 'happy'});
  const [kind, text, options] = ctx.buddy.log[0];
  assert.equal(kind, 'say');
  assert.equal(text, 'Задача идёт дольше плана');
  assert.equal(options.detail, 'на 2 дня');
  assert.equal(options.queue, true);
  assert.equal(options.ms, 0);
  assert.equal(options.mood, 'happy');
  assert.deepEqual(options.buttons.map(b => b.label), ['Обсудить с Бади', 'Понятно']);
  const done = {message: {source: 'ai-buddy-extension', op: 'done', id: 'n1'}, origin};
  options.buttons[0].onClick();
  assert.deepEqual(ctx.posted.at(-1), done);
  assert.deepEqual(ctx.buddy.log.at(-1), ['ask', 'Как ускориться?']);
  options.buttons[1].onClick();
  assert.equal(ctx.posted.length, 2);
  options.onClick();
  assert.equal(ctx.posted.length, 3);
  assert.deepEqual(ctx.buddy.log.at(-1), ['ask', 'Как ускориться?']);

  ctx.buddy.log.length = 0;
  ctx.fromSite({source: 'ai-buddy-web', op: 'say', text: 'Наставник ответил', mood: 'weird', id: 7});
  const [, , plain] = ctx.buddy.log[0];
  assert.equal(plain.mood, 'waving');
  assert.deepEqual(plain.buttons.map(b => b.label), ['Понятно']);
  plain.onClick();
  assert.equal(ctx.buddy.log.at(-1), 'open');
  assert.deepEqual(ctx.posted.at(-1).message, {source: 'ai-buddy-extension', op: 'done', id: ''});
});

test('a hidden Бади is not brought back by the site', () => {
  ctx.buddy.shown = false;
  ctx.buddy.log.length = 0;
  ctx.fromSite({source: 'ai-buddy-web', op: 'ask', question: 'x'});
  ctx.fromSite({source: 'ai-buddy-web', op: 'say', text: 'x'});
  assert.deepEqual(ctx.buddy.log, []);
});
