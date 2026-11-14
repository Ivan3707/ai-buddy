import assert from 'node:assert/strict';
import test from 'node:test';
import { applyStreamEvent, createAssistantState, normalizeForm } from '../lib/stream-events.js';
import { toView } from '../lib/buddy.js';

const event = (type, data) => ({type, data: JSON.stringify(data)});

test('normalizeForm keeps only known form types and string values', () => {
  assert.deepEqual(normalizeForm({type: 'note', prefill: {title: 'Идея', evil: 5, content: {x: 1}}}), {type: 'note', prefill: {title: 'Идея'}});
  assert.equal(normalizeForm({type: 'drop_tables', prefill: {}}), null);
  assert.equal(normalizeForm(null), null);
  assert.deepEqual(normalizeForm({type: 'event'}), {type: 'event', prefill: {}});
});

test('a form event reaches the view model and does not finish the stream', () => {
  let state = createAssistantState();
  state = applyStreamEvent(state, event('delta', {text: 'Заполни форму ниже.'}));
  state = applyStreamEvent(state, event('form', {form: {type: 'leave_request', prefill: {kind: 'day_off', date_from: '2026-11-21'}}}));
  assert.equal(state.finished, false);
  assert.deepEqual(toView(state).form, {type: 'leave_request', prefill: {kind: 'day_off', date_from: '2026-11-21'}});
  state = applyStreamEvent(state, event('done', {finish_reason: 'form_shown'}));
  assert.equal(toView(state).form.type, 'leave_request');
  assert.equal(state.finishReason, 'form_shown');
});

test('an unusable form event is ignored', () => {
  const state = applyStreamEvent(createAssistantState(), event('form', {form: {type: 'nope', prefill: {}}}));
  assert.equal(toView(state).form, null);
});
