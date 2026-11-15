import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  clampPosition, displayName, moodOf, pageHint, requestStatusChanges, siteMatchPattern, toView,
} from '../lib/buddy.js';
import { applyStreamEvent, createAssistantState, finalizeStream } from '../lib/stream-events.js';

const ev = (type, data) => ({type, data: JSON.stringify(data)});

test('page hints come from the title/url only and pick the right topic', () => {
  assert.match(pageHint({title: 'Grafana — Рекомендации (демо)'}).question, /смотреть/);
  assert.match(pageHint({url: 'https://jira.example/browse/REC-1'}).text, /Jira/);
  assert.match(pageHint({title: 'Merge request !42'}).question, /ревью/);
  const wiki = pageHint({url: 'https://wiki.example/code-review'});
  assert.ok(wiki); // "code-review" matches the review hint first: still a hint, never null
  assert.equal(pageHint({title: 'Вики: регламенты'}).question, null); // wiki hint asks to select text
  assert.equal(pageHint({title: 'Новости', url: 'https://news.example/'}), null);
  assert.equal(pageHint(), null);
});

test('request status changes: first call only remembers, later calls report changes', () => {
  const first = requestStatusChanges(undefined, [{number: 'ABS-000001', status: 'pending_approval'}]);
  assert.deepEqual(first.changes, []);
  const second = requestStatusChanges(first.next, [
    {number: 'ABS-000001', status: 'approved'},
    {number: 'ACC-000002', status: 'pending_approval'}, // new request: not a status change
    {number: 1, status: 'x'}, null,
  ]);
  assert.deepEqual(second.changes, [{number: 'ABS-000001', status: 'approved', text: 'Заявка ABS-000001: согласована'}]);
  assert.deepEqual(second.next, {'ABS-000001': 'approved', 'ACC-000002': 'pending_approval'});
  const odd = requestStatusChanges(second.next, [{number: 'ACC-000002', status: 'archived'}]);
  assert.equal(odd.changes[0].text, 'Заявка ACC-000002: archived');
  assert.deepEqual(requestStatusChanges({}, 'not a list'), {changes: [], next: {}});
});

test('mood follows the answer: thinking -> talking -> waiting / sad / idle', () => {
  let state = createAssistantState();
  assert.equal(moodOf(null), 'idle');
  assert.equal(moodOf(state), 'thinking');
  state = applyStreamEvent(state, ev('delta', {text: 'Привет'}));
  assert.equal(moodOf(state), 'talking');
  const withCard = applyStreamEvent(state, ev('action', {action: {id: 'a1', type: 'leave_request', summary: 'Отпуск', params: {kind: 'vacation'}}}));
  assert.equal(moodOf(applyStreamEvent(withCard, ev('done', {}))), 'waiting');
  assert.equal(moodOf(applyStreamEvent(state, ev('done', {}))), 'idle');
  assert.equal(moodOf(finalizeStream(state, 'network')), 'sad');
});

test('view model: display strings only, unsafe sources dropped, feedback id only on success', () => {
  let state = createAssistantState();
  state = applyStreamEvent(state, ev('meta', {conversation_id: 'c', message_id: 'm1'}));
  state = applyStreamEvent(state, ev('delta', {text: 'Ответ [1]'}));
  state = applyStreamEvent(state, ev('sources', {sources: [
    {id: 'leave', title: 'Отпуска', url: 'https://kb.example/leave', snippet: 'за 14 дней'},
    {id: 'bad', title: 'x', url: 'javascript:alert(1)'},
  ]}));
  state = applyStreamEvent(state, ev('action', {action: {
    id: 'a1', type: 'leave_request', summary: 'Отпуск 01.12–14.12', status: 'pending',
    params: {kind: 'vacation', date_from: '2026-12-01', date_to: '2026-12-14'},
  }}));
  const streaming = toView(state);
  assert.equal(streaming.messageId, null);
  assert.equal(streaming.status, 'Ищу в базе знаний…');
  state = applyStreamEvent(state, ev('done', {}));
  const view = toView(state);
  assert.equal(view.messageId, 'm1');
  assert.deepEqual(view.sources, [{id: 'leave', title: 'Отпуска', url: 'https://kb.example/leave', snippet: 'за 14 дней'}]);
  assert.equal(view.card.title, 'Заявка на отсутствие');
  assert.deepEqual(view.card.rows.map(row => row.value), ['Отпуск', '01.12.2026', '14.12.2026']);
  assert.equal(view.card.pending, true);
  assert.equal(view.error, null);
  const failed = toView(finalizeStream(createAssistantState(), 'rate_limited'));
  assert.equal(failed.error, 'Слишком много запросов. Подождите немного.');
  assert.equal(failed.card, null);
  const done = toView(applyStreamEvent(createAssistantState(), ev('action', {action: {
    id: 'a2', type: 'mentor_question', summary: 'Вопрос', status: 'executed', params: {question: 'Q'},
  }})));
  assert.equal(done.card.pending, false);
  assert.equal(done.card.result, 'Выполнено.');
});

test('drag position is clamped to the viewport', () => {
  const viewport = {width: 1000, height: 600};
  assert.deepEqual(clampPosition({x: -50, y: 900}, viewport, 72), {x: 8, y: 520});
  assert.deepEqual(clampPosition({x: 400.4, y: 300.6}, viewport, 72), {x: 400, y: 301});
  assert.deepEqual(clampPosition({x: NaN, y: undefined}, viewport, 72), {x: 920, y: 520});
  assert.deepEqual(clampPosition({x: 50, y: 50}, {width: 40, height: 40}, 72), {x: 8, y: 8});
});

test('site match pattern: per host, http(s) only', () => {
  assert.equal(siteMatchPattern('https://wiki.example.com/a/b?x=1#y'), 'https://wiki.example.com/*');
  assert.equal(siteMatchPattern('http://127.0.0.1:5173/demo/wiki.html'), 'http://127.0.0.1/*');
  assert.equal(siteMatchPattern('chrome://extensions'), null);
  assert.equal(siteMatchPattern('not a url'), null);
});

test('display name: profile name, a real-looking email name, or nothing', () => {
  assert.equal(displayName({display_name: ' Аня '}), 'Аня');
  assert.equal(displayName({email: 'masha@example.com'}), 'masha');
  assert.equal(displayName({email: 'intern@demo.local'}), '');
  assert.equal(displayName({email: 'a.b-1@x'}), '');
  assert.equal(displayName(null), '');
});

test('bubble layout stays inside the window and moves up near the bottom edge', async () => {
  await import('../buddy/widget.js');
  const {bubbleLayout} = globalThis.AIBuddyWidget;
  const view = {w: 1000, h: 700};
  const low = bubbleLayout({pos: {x: 900, y: 620}, size: 72, view, bubble: {w: 300, h: 300}});
  assert.ok(low.y + low.h <= view.h - 8 && low.y >= 8);
  assert.ok(low.x + low.w < 900); // character on the right half: bubble to his left
  const tall = bubbleLayout({pos: {x: 20, y: 600}, size: 72, view, bubble: {w: 300, h: 2000}});
  assert.equal(tall.h, 684);
  assert.equal(tall.y, 8);
  assert.ok(tall.x >= 8 && tall.x + tall.w <= view.w - 8);
  const tiny = bubbleLayout({pos: {x: 0, y: 0}, size: 72, view: {w: 200, h: 200}, bubble: {w: 300, h: 50}});
  assert.equal(tiny.w, 184);
  assert.equal(tiny.x, 8);
});
