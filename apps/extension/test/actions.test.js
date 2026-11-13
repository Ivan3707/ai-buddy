import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  actionTitle, describeActionError, describeActionStatus, describeParams, describeResult, formatDate,
} from '../lib/actions.js';
import { cancelAction, confirmAction, HttpError, postJson } from '../lib/api.js';
import { fakeFetch, jsonResponse, textResponse } from './helpers.js';

const base = 'http://127.0.0.1:8080';

test('titles and dates', () => {
  assert.equal(actionTitle('leave_request'), 'Заявка на отсутствие');
  assert.equal(actionTitle('mentor_question'), 'Вопрос наставнику');
  assert.equal(actionTitle('???'), 'Действие');
  assert.equal(formatDate('2026-11-20'), '20.11.2026');
  assert.equal(formatDate('завтра'), 'завтра');
  assert.equal(formatDate(undefined), '');
});

test('describeParams for every action type', () => {
  assert.deepEqual(describeParams('leave_request', {kind: 'day_off', date_from: '2026-11-20', date_to: '2026-11-20'}), [
    {label: 'Вид', value: 'Отгул'},
    {label: 'С', value: '20.11.2026'},
    {label: 'По', value: '20.11.2026'},
  ]);
  assert.deepEqual(describeParams('leave_request', {kind: 'sabbatical', comment: 'семейное'}), [
    {label: 'Вид', value: 'sabbatical'},
    {label: 'Комментарий', value: 'семейное'},
  ]);
  assert.deepEqual(describeParams('access_request', {system: 'grafana', resource: 'payments', access_level: 'read', justification: 'онбординг'}), [
    {label: 'Система', value: 'Grafana'},
    {label: 'Ресурс', value: 'payments'},
    {label: 'Уровень', value: 'чтение'},
    {label: 'Обоснование', value: 'онбординг'},
  ]);
  assert.deepEqual(describeParams('access_request', {system: 'vault', access_level: 'root'}), [
    {label: 'Система', value: 'vault'},
    {label: 'Уровень', value: 'root'},
  ]);
  assert.deepEqual(describeParams('mentor_question', {question: 'Где регламент?', context: ''}), [{label: 'Вопрос', value: 'Где регламент?'}]);
  assert.deepEqual(describeParams('future', {a: 1, b: {c: 2}, d: null}), [
    {label: 'a', value: '1'},
    {label: 'b', value: '{"c":2}'},
  ]);
  assert.deepEqual(describeParams('mentor_question'), []);
});

test('describeResult covers request, mentor question and plain statuses', () => {
  assert.deepEqual(
    describeResult({action: {status: 'executed'}, request: {number: 'ABS-000123', status: 'pending_approval', approver: 'Иванова А.'}}),
    {tone: 'success', text: 'Заявка ABS-000123 создана, статус: на согласовании. Согласующий: Иванова А.'},
  );
  assert.equal(describeResult({request: {number: 'ACC-1', status: 'approved'}}).text, 'Заявка ACC-1 создана, статус: согласована.');
  assert.equal(describeResult({request: {number: 'ACC-2'}}).text, 'Заявка ACC-2 создана, статус: создана.');
  assert.equal(describeResult({action: {status: 'executed'}, mentor_question: {id: 'q'}}).text.startsWith('Вопрос передан наставнику'), true);
  assert.deepEqual(describeResult({action: {status: 'failed', error: 'нет квоты'}}), {tone: 'error', text: 'Не удалось выполнить: нет квоты'});
  assert.deepEqual(describeResult({action: {status: 'cancelled'}}), {tone: 'muted', text: 'Отменено.'});
  assert.deepEqual(describeResult({action: {status: 'executed'}}), {tone: 'success', text: 'Выполнено.'});
  assert.deepEqual(describeResult(undefined), {tone: 'success', text: 'Выполнено.'});
});

test('describeActionStatus', () => {
  assert.equal(describeActionStatus({status: 'executed'}), 'Выполнено.');
  assert.equal(describeActionStatus({status: 'cancelled'}), 'Отменено.');
  assert.equal(describeActionStatus({status: 'failed'}), 'Не удалось выполнить.');
  assert.equal(describeActionStatus({status: 'pending'}), '');
  assert.equal(describeActionStatus(null), '');
});

test('describeActionError maps 404/409/422 as final and others as retryable', () => {
  assert.deepEqual(describeActionError(404), {text: 'Действие не найдено или недоступно.', retryable: false});
  assert.equal(describeActionError(409, 'confirm').text, 'Действие уже отменено.');
  assert.equal(describeActionError(409, 'cancel').text, 'Действие уже выполнено — отменить нельзя.');
  assert.equal(describeActionError(422).retryable, false);
  assert.equal(describeActionError(500).retryable, true);
  assert.equal(describeActionError(undefined).retryable, true);
});

test('confirmAction and cancelAction POST to the action endpoints with auth', async () => {
  const fetchImpl = fakeFetch(
    jsonResponse({action: {id: 'a/1', status: 'executed'}, request: {number: 'ABS-1'}}),
    jsonResponse({id: 'a/1', status: 'cancelled'}),
  );
  const options = {baseUrl: base, token: 'jwt', fetchImpl};
  const confirmed = await confirmAction(options, 'a/1');
  assert.equal(confirmed.request.number, 'ABS-1');
  assert.equal(fetchImpl.calls[0].url, `${base}/api/actions/a%2F1/confirm`);
  assert.equal(fetchImpl.calls[0].init.headers.Authorization, 'Bearer jwt');
  assert.equal(fetchImpl.calls[0].init.body, undefined);
  const cancelled = await cancelAction(options, 'a/1');
  assert.equal(cancelled.status, 'cancelled');
  assert.equal(fetchImpl.calls[1].url, `${base}/api/actions/a%2F1/cancel`);
});

test('postJson: JSON body, 204, empty 200 and errors', async () => {
  const fetchImpl = fakeFetch(
    new Response(null, {status: 204}),
    new Response('', {status: 200}),
    jsonResponse({error: 'action cancelled'}, 409),
    textResponse('oops', 500),
  );
  const options = {baseUrl: base, token: 't', fetchImpl, path: '/x'};
  assert.equal(await postJson({...options, body: {rating: 'up'}}), undefined);
  assert.equal(fetchImpl.calls[0].init.headers['Content-Type'], 'application/json');
  assert.deepEqual(fetchImpl.calls[0].body, {rating: 'up'});
  assert.equal(await postJson(options), undefined);
  await assert.rejects(postJson(options), error => error instanceof HttpError && error.status === 409 && error.message === 'action cancelled');
  await assert.rejects(postJson(options), error => error.status === 500 && error.message === 'Backend: HTTP 500');
});

test('postJson notifies about 401', async () => {
  let notified = 0;
  globalThis.dispatchEvent = event => { if (event.type === 'ai-buddy:unauthorized') notified += 1; };
  try {
    const fetchImpl = fakeFetch(jsonResponse({error: 'unauthorized'}, 401));
    await assert.rejects(postJson({baseUrl: base, token: 't', fetchImpl, path: '/x'}), HttpError);
    assert.equal(notified, 1);
  } finally {
    delete globalThis.dispatchEvent;
  }
});
