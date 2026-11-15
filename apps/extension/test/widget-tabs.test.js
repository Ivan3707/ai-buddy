// buddy/widget.js, full mode tabs: Чат, Вопросы наставнику, Заявки, Заметки, Календарь.
import assert from 'node:assert/strict';
import { afterEach, beforeEach, mock, test } from 'node:test';
import { flush } from './dom.js';
import { mountWidget, page, submit, tick } from './widget-harness.js';

let current = null;
beforeEach(() => { mock.timers.enable({apis: ['setTimeout', 'setInterval']}); });
afterEach(() => { current?.api.remove(); current = null; mock.timers.reset(); });

async function full(handlers, env) {
  const w = current = await mountWidget({handlers, env});
  await w.open();
  w.$('.mode').click();
  await flush();
  return w;
}
const tab = async (w, name) => { w.$(`.tabs [data-tab=${name}]`).click(); await flush(); };
const panels = w => ({
  chat: !w.$('.logwrap').hidden, notes: !w.$('.notes').hidden, reqs: !w.$('.reqs').hidden, ask: !w.$('.ask').hidden, cal: !w.$('.cal').hidden,
});

test('tabs show one panel at a time and mark the current one; compact mode always returns to the chat', async () => {
  const w = await full({notes: {notes: []}, 'requests-list': {requests: []}, questions: {questions: []}, events: {events: []}});
  assert.deepEqual(panels(w), {chat: true, notes: false, reqs: false, ask: false, cal: false});
  for (const name of ['notes', 'reqs', 'ask', 'cal']) {
    await tab(w, name);
    assert.equal(panels(w)[name], true);
    assert.equal(Object.values(panels(w)).filter(Boolean).length, 1);
    const button = w.$(`.tabs [data-tab=${name}]`);
    assert.ok(button.classList.contains('on'));
    assert.equal(button.getAttribute('aria-selected'), 'true');
    assert.equal(w.$('.composer-wrap').hidden, true);
  }
  assert.deepEqual(w.ops().filter(op => ['notes', 'requests-list', 'questions', 'events'].includes(op)), ['notes', 'requests-list', 'questions', 'events']);
  await tab(w, 'chat');
  assert.equal(panels(w).chat, true);
  assert.equal(w.$('.composer-wrap').hidden, false);

  await tab(w, 'notes');
  w.$('.mode').click();
  assert.equal(panels(w).chat, true, 'the small window has no tabs');
  assert.equal(w.$('.tabs').hidden, true);

  // asking from another tab switches to the chat
  w.$('.mode').click();
  await tab(w, 'cal');
  await w.api.ask('вопрос');
  await flush();
  assert.equal(panels(w).chat, true);
});

test('notes: empty list, create, select, edit, errors', async () => {
  let notes = [];
  const saves = [{error: 'Введите заголовок.'}, {note: {id: 'n1'}}, undefined, {note: {id: 'n1'}}];
  const w = await full({
    notes: () => ({notes}),
    'note-save': message => {
      const r = saves.shift();
      if (r?.note) notes = [{id: 'n1', title: message.title, content: message.content, updated_at: '2026-03-04T10:00:00Z'}, {id: 'n2', title: '', content: 'x', updated_at: 'bad'}];
      return r;
    },
  });
  await tab(w, 'notes');
  assert.equal(w.$('.notes .items').textContent, 'Заметок пока нет.');

  const title = w.$('.n-title'), text = w.$('.n-text'), status = w.$('.n-status'), save = w.$('.n-save');
  save.click();
  assert.equal(status.textContent, 'Сохраняю…');
  assert.equal(save.disabled, true);
  await flush();
  assert.equal(status.textContent, 'Введите заголовок.');
  assert.equal(save.disabled, false);

  title.value = 'Пароли'; text.value = 'спросить у админа';
  save.click();
  await flush();
  assert.deepEqual(w.calls.filter(c => c.op === 'note-save').at(-1), {op: 'note-save', id: undefined, title: 'Пароли', content: 'спросить у админа'});
  assert.equal(status.textContent, 'Сохранено');
  const items = w.$$('.notes .items .item');
  assert.deepEqual(items.map(b => b.querySelector('b').textContent), ['Пароли', 'Без названия']);
  assert.equal(items[0].querySelector('small').textContent, '04.03');
  assert.ok(items[0].classList.contains('on'), 'the saved note is selected');

  w.$('.new-note').click();
  assert.equal(title.value, '');
  assert.equal(w.$$('.notes .items .item.on').length, 0);
  w.$$('.notes .items .item')[0].click();
  assert.equal(title.value, 'Пароли');
  assert.equal(text.value, 'спросить у админа');
  title.value = 'Пароли и доступы';
  save.click();
  await flush();
  assert.equal(status.textContent, 'Не удалось сохранить.');
  save.click();
  await flush();
  assert.equal(w.calls.filter(c => c.op === 'note-save').at(-1).id, 'n1', 'an existing note is updated');

  // a failed reload shows the error
  w.handlers.notes = {error: 'Сессия истекла. Обновите страницу.'};
  await tab(w, 'notes');
  assert.equal(status.textContent, 'Сессия истекла. Обновите страницу.');
  w.handlers.notes = undefined;
  await tab(w, 'notes');
  assert.equal(status.textContent, '');
});

test('requests: list with statuses and portal links; the form switches fields, validates and sends', async () => {
  let list = [
    {kind: 'assistant', id: 'r1', number: 'REQ-1', status: 'pending_approval', type: 'leave_request', params: {kind: 'day_off', date_from: '2026-12-01', date_to: '2026-12-01', comment: 'дела'}, approver: 'Иванов'},
    {kind: 'assistant', id: 'r2', number: 'REQ-2', status: 'approved', type: 'leave_request', params: {kind: 'vacation', date_from: '2026-12-01', date_to: '2026-12-10'}},
    {kind: 'assistant', id: 'r3', number: 'REQ-3', status: 'rejected', type: 'access_request', params: {system: 'grafana', resource: 'billing', access_level: 'write', justification: 'дежурство'}},
    {kind: 'corporate', id: 'c1', number: 'ACC-1', status: 'weird', type: 'corporate_access', params: {tool: 'jira'}},
    {kind: 'assistant', id: 'r4', number: 'REQ-4', status: 'approved', type: 'leave_request'},
    {kind: 'assistant', id: 'r5', number: 'REQ-5', status: 'approved', type: 'access_request', params: {access_level: 'owner'}},
  ];
  const creates = [{error: 'Нет прав'}, undefined, {request: {id: 'r9', number: 'REQ-9', status: 'pending_approval'}}, {request: {id: 'r10', number: 'REQ-10', status: 'custom'}}];
  const w = await full({'requests-list': () => ({requests: list}), 'request-create': () => creates.shift()},
    {portalUrl: (kind, id) => (kind === 'corporate' ? '' : `https://portal/${kind}/${id}`)});
  await tab(w, 'reqs');
  const rows = w.$$('.r-list > *');
  assert.equal(rows.length, 6);
  assert.equal(rows[0].tagName, 'A');
  assert.equal(rows[0].href, 'https://portal/assistant/r1');
  assert.equal(rows[0].querySelector('.pill').textContent, 'на согласовании');
  assert.match(rows[0].textContent, /Отгул: 01\.12\.2026/);
  assert.match(rows[0].textContent, /Согласует: Иванов/);
  assert.match(rows[0].textContent, /дела/);
  assert.match(rows[1].textContent, /Отпуск: 01\.12\.2026 — 10\.12\.2026/);
  assert.match(rows[2].textContent, /Доступ: grafana · billing · изменение/);
  assert.equal(rows[2].querySelector('.pill').textContent, 'отклонена');
  assert.match(rows[2].textContent, /дежурство/);
  assert.equal(rows[3].tagName, 'DIV', 'no portal page: a plain row');
  assert.match(rows[3].textContent, /Доступ к инструменту: jira/);
  assert.equal(rows[3].querySelector('.pill').textContent, 'weird');
  assert.match(rows[4].textContent, /Отсутствие: /);
  assert.match(rows[5].textContent, /Доступ:  ·  · owner/);

  const form = w.$('.r-form');
  assert.equal(form.hidden, true);
  w.$('.r-new').click();
  assert.equal(form.hidden, false);
  assert.ok(form.date_from.value);
  assert.equal(form.date_from.value, form.date_to.value);
  assert.equal(w.$('.f-access').hidden, true);

  form.date_from.value = '';
  submit(form);
  assert.equal(w.$('.r-status').textContent, 'Укажите даты.');

  form.type.value = 'access_request';
  form.type.dispatchEvent(new page.window.Event('change'));
  assert.equal(w.$('.f-access').hidden, false);
  assert.equal(w.$('.f-leave').hidden, true);
  submit(form);
  assert.equal(w.$('.r-status').textContent, 'Укажите ресурс и зачем нужен доступ.');
  form.resource.value = 'billing';
  form.justification.value = 'дежурство';
  submit(form);
  assert.equal(w.$('.r-status').textContent, 'Отправляю…');
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'request-create', type: 'access_request', params: {system: 'jira', resource: 'billing', access_level: 'read', justification: 'дежурство'}});
  assert.equal(w.$('.r-status').textContent, 'Нет прав');
  submit(form);
  await flush();
  assert.equal(w.$('.r-status').textContent, 'Не удалось создать заявку.');

  list = [];
  submit(form);
  await flush();
  assert.equal(form.hidden, true);
  assert.equal(w.$('.r-list .notice').textContent, 'Заявка REQ-9 создана, статус: на согласовании.');
  assert.match(w.$('.r-list').textContent, /Заявок пока нет/);
  await tick(8000);
  assert.equal(w.$('.r-list .notice'), null, 'the notice goes away');

  w.$('.r-new').click();
  form.comment.value = 'коротко';
  form.date_to.value = '';
  submit(form);
  await flush();
  const leave = w.calls.filter(c => c.op === 'request-create').at(-1);
  assert.equal(leave.type, 'leave_request');
  assert.equal(leave.params.kind, 'day_off');
  assert.equal(leave.params.date_to, leave.params.date_from);
  assert.equal(w.$('.r-list .notice').textContent, 'Заявка REQ-10 создана, статус: custom.');

  w.$('.r-new').click();
  w.$('.r-cancel').click();
  assert.equal(form.hidden, true);

  w.handlers['requests-list'] = {error: 'Сессия истекла. Обновите страницу.'};
  await tab(w, 'reqs');
  assert.equal(w.$('.r-list .err').textContent, 'Сессия истекла. Обновите страницу.');
  w.handlers['requests-list'] = undefined;
  await tab(w, 'reqs');
  assert.equal(w.$('.r-list .err').textContent, 'Не удалось загрузить заявки.');
});

test('questions to the mentor: list with answers, empty question, send, errors', async () => {
  let list = [];
  const creates = [undefined, {error: 'Слишком длинно'}, {question: {id: 'q1'}}];
  const w = await full({questions: () => ({questions: list}), 'question-create': () => {
    const r = creates.shift();
    if (r?.question) list = [{question: 'Кто ревьюит?', status: 'answered', answer: 'Петров', created_at: '2026-03-04T10:00:00Z'}, {question: 'Где VPN?', status: 'open', created_at: '2026-03-05T10:00:00Z'}, {question: '?', status: 'other', created_at: 'x'}];
    return r;
  }});
  await tab(w, 'ask');
  assert.match(w.$('.q-list').textContent, /Вопросов наставнику пока нет/);
  const form = w.$('.q-form'), status = w.$('.q-status');
  submit(form);
  assert.equal(status.textContent, 'Напишите вопрос.');
  form.question.value = 'Кто ревьюит?';
  submit(form);
  assert.equal(status.textContent, 'Отправляю…');
  await flush();
  assert.equal(status.textContent, 'Не удалось отправить.');
  submit(form);
  await flush();
  assert.equal(status.textContent, 'Слишком длинно');
  submit(form);
  await flush();
  assert.deepEqual(w.calls.filter(c => c.op === 'question-create').at(-1), {op: 'question-create', question: 'Кто ревьюит?'});
  assert.equal(status.textContent, 'Вопрос передан наставнику.');
  assert.equal(form.question.value, '');
  const rows = w.$$('.q-list .req');
  assert.equal(rows.length, 3);
  assert.equal(rows[0].querySelector('.pill').textContent, 'отвечен');
  assert.equal(rows[0].querySelector('.answer div').textContent, 'Петров');
  assert.equal(rows[1].querySelector('.pill').textContent, 'ждёт ответа');
  assert.equal(rows[1].querySelector('.answer'), null);
  assert.equal(rows[2].querySelector('.pill').textContent, 'other');

  w.handlers.questions = undefined;
  await tab(w, 'ask');
  assert.equal(w.$('.q-list .err').textContent, 'Не удалось загрузить вопросы.');
});

test('calendar: upcoming first, then the past; adding an event', async () => {
  const hour = 3600000, now = Date.now();
  const iso = t => new Date(t).toISOString();
  let list = [];
  const creates = [{error: 'Окончание должно быть позже начала.'}, undefined, {event: {id: 'e1'}}];
  const w = await full({events: () => ({events: list}), 'event-create': () => {
    const r = creates.shift();
    if (r?.event) {
      list = [
        {title: 'Позже', starts_at: iso(now + 5 * hour), ends_at: iso(now + 6 * hour)},
        {title: 'Скоро', starts_at: iso(now + hour), ends_at: iso(now + 2 * hour), description: 'созвон'},
        {title: 'Вчера', starts_at: iso(now - 26 * hour), ends_at: iso(now - 25 * hour)},
      ];
    }
    return r;
  }});
  await tab(w, 'cal');
  assert.equal(w.$('.e-list').textContent, 'Событий пока нет.');
  const form = w.$('.e-form'), status = w.$('.e-status');
  assert.match(form.starts_at.value, /^\d{4}-\d{2}-\d{2}T\d{2}:00$/);
  assert.ok(form.ends_at.value > form.starts_at.value);
  form.elements.namedItem('title').value = 'Встреча';
  submit(form);
  assert.equal(status.textContent, 'Сохраняю…');
  await flush();
  assert.equal(status.textContent, 'Окончание должно быть позже начала.');
  const sent = w.calls.at(-1);
  assert.equal(sent.op, 'event-create');
  assert.equal(sent.title, 'Встреча');
  assert.equal(sent.starts_at, form.starts_at.value);
  submit(form);
  await flush();
  assert.equal(status.textContent, 'Не удалось создать событие.');
  submit(form);
  await flush();
  assert.equal(status.textContent, 'Событие добавлено.');
  assert.equal(form.elements.namedItem('title').value, '', 'the form is reset');
  const titles = w.$$('.e-list .req b').map(b => b.textContent);
  assert.deepEqual(titles, ['Скоро', 'Позже', 'Вчера']);
  assert.ok(w.$$('.e-list .req')[2].classList.contains('dim'));
  assert.equal(w.$('.e-list small.pad').textContent, 'Прошедшие');
  assert.equal(w.$$('.e-list .req')[0].querySelector('small').textContent, 'созвон');

  w.handlers.events = {error: 'Нет связи'};
  await tab(w, 'cal');
  assert.equal(w.$('.e-list .err').textContent, 'Нет связи');
  w.handlers.events = undefined;
  await tab(w, 'cal');
  assert.equal(w.$('.e-list .err').textContent, 'Не удалось загрузить календарь.');
});
