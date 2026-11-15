// buddy/widget.js: the character, the bubble, sign-in, the chat with streamed answers, cards, forms, feedback,
// sources and the document reader, page context and the selection hint. Driven like a user would, in jsdom.
import assert from 'node:assert/strict';
import { afterEach, beforeEach, mock, test } from 'node:test';
import { flush } from './dom.js';
import { key, mountWidget, page, pointer, submit, tick } from './widget-harness.js';

let current = null;
async function mount(options) { current = await mountWidget(options); return current; }

beforeEach(() => { mock.timers.enable({apis: ['setTimeout', 'setInterval']}); });
afterEach(() => {
  current?.api.remove();
  current = null;
  mock.timers.reset();
  document.title = 'Портал';
  document.body.replaceChildren();
  page.select('');
});

const visible = node => !node.hidden;

test('mount: the character is placed at the stored position (share of the window) and can be hidden/shown', async () => {
  const w = await mount({stored: {buddyPos: {fx: 0.5, fy: 0.5}}});
  assert.equal(w.api.isShown(), true);
  assert.ok(w.api.host.isConnected);
  assert.equal(w.api.host.dataset.buddyX, String(Math.round(0.5 * (innerWidth - 72))));
  assert.equal(w.api.host.dataset.buddyY, String(Math.round(0.5 * (innerHeight - 72))));
  assert.deepEqual(w.api.position(), {x: Math.round(0.5 * (innerWidth - 72)), y: Math.round(0.5 * (innerHeight - 72)), size: 72});
  assert.equal(w.$('.char').dataset.mood, 'idle');
  assert.equal(w.api.isOpen(), false);
  w.api.hide();
  assert.equal(w.api.isShown(), false);
  w.api.show();
  assert.equal(w.api.isShown(), true);
  // an older version stored pixels; syncPosition() follows a change made in another window
  w.data.set('buddyPos', {x: 100, y: 120});
  await w.api.syncPosition();
  assert.deepEqual(w.api.position(), {x: 100, y: 120, size: 72});
  w.data.set('buddyPos', null);
  await w.api.syncPosition();
  assert.deepEqual(w.api.position(), {x: 100, y: 120, size: 72});
});

test('a window resize keeps the character inside the window', async () => {
  const w = await mount({stored: {buddyPos: {x: 900, y: 600}}});
  page.window.innerWidth = 500;
  page.window.dispatchEvent(new page.window.Event('resize'));
  assert.ok(Number(w.api.host.dataset.buddyX) <= 500 - 72 - 8);
  page.window.innerWidth = 1024;
  page.window.dispatchEvent(new page.window.Event('resize'));
});

test('signed out: the login form; a wrong password shows the error, a good one opens the chat', async () => {
  let authed = false;
  const results = [{ok: false, error: 'Неверный email или пароль.'}, {ok: true}];
  const w = await mount({handlers: {
    session: () => (authed ? {authenticated: true, user: {id: 'u1', name: 'Аня'}} : {authenticated: false}),
    login: () => { const r = results.shift(); authed = r.ok; return r; },
    suggestions: {suggestions: ['Где взять VPN?', 'Как оформить отпуск?']},
  }});
  await w.open();
  assert.equal(w.api.isOpen(), true);
  assert.ok(visible(w.$('.login')));
  assert.equal(w.$('.composer-wrap').hidden, true);
  assert.equal(w.$('.tabs').hidden, true);

  const form = w.$('.login');
  form.email.value = 'anya@example.com';
  form.password.value = 'secret';
  submit(form);
  await flush();
  assert.equal(w.$('.login-error').textContent, 'Неверный email или пароль.');
  assert.equal(form.password.value, '', 'the password is never kept in the field');
  assert.deepEqual(w.calls.find(c => c.op === 'login'), {op: 'login', email: 'anya@example.com', password: 'secret'});

  form.password.value = 'right';
  submit(form);
  await flush();
  assert.equal(w.$('.login-error').textContent, '');
  assert.equal(w.$('.login').hidden, true);
  assert.ok(visible(w.$('.composer-wrap')));
  assert.ok(['Где взять VPN?', 'Как оформить отпуск?'].includes(w.$('.composer textarea').placeholder));
  assert.match(w.$('.composer textarea').title, /Enter на пустом поле/);
});

test('login fails without a reason, the backend is down, or the host has no login (session expired)', async () => {
  const w = await mount({handlers: {session: {authenticated: false, unavailable: true}, login: undefined}});
  await w.open();
  assert.equal(w.$('.login-error').textContent, 'Backend недоступен. Проверьте, что стенд запущен.');
  submit(w.$('.login'));
  await flush();
  assert.equal(w.$('.login-error').textContent, 'Не удалось войти.');
  w.api.remove();

  const noLogin = await mount({handlers: {session: {authenticated: false}}, features: {login: false}});
  await noLogin.open();
  assert.equal(noLogin.$('.login').hidden, true);
  assert.ok(visible(noLogin.$('.expired')));
});

test('a streamed answer: status, markdown text, sources, feedback; the mood follows', async () => {
  const w = await mount();
  await w.open();
  await w.send('Привет');
  const s = w.lastStream();
  assert.deepEqual(s.payload, {op: 'chat', message: 'Привет', pageContext: null});
  assert.equal(w.$('.msg.user').textContent, 'Привет');
  assert.equal(w.$('.send').disabled, true);

  s.view({status: 'Ищу в базе знаний…', mood: 'thinking'});
  const bot = w.$('.msg.bot');
  assert.equal(bot.querySelector('.status').textContent, 'Ищу в базе знаний…');
  assert.equal(w.$('.char').dataset.mood, 'thinking');

  s.view({text: '## План\n**Важно** и `vpn` [1]\n- шаг один\n* шаг два\nконец <script>', mood: 'talking'});
  const text = bot.querySelector('.text');
  assert.equal(text.querySelector('h4').textContent, 'План');
  assert.equal(text.querySelector('b').textContent, 'Важно');
  assert.equal(text.querySelector('code').textContent, 'vpn');
  assert.equal(text.querySelector('sup').textContent, '[1]');
  assert.deepEqual([...text.querySelectorAll('li')].map(li => li.textContent), ['шаг один', 'шаг два']);
  assert.equal(text.querySelector('script'), null, 'answer text is escaped');
  assert.match(text.textContent, /конец <script>/);
  assert.equal(w.$('.char').dataset.mood, 'talking');

  s.view({text: 'Готово\n- a\n- b', finished: true, mood: 'idle', messageId: 'm1',
    sources: [{id: 'kb-1', title: 'VPN', url: 'https://wiki.example.com/vpn', snippet: 'Установите клиент'},
      {id: 'kb-2', title: 'Сеть', url: 'https://wiki.example.com/net', refs: [2, 4]}]});
  assert.equal(bot.querySelector('.status').hidden, true);
  assert.equal(bot.querySelectorAll('.text li').length, 2);
  const toggle = bot.querySelector('.src-toggle');
  assert.equal(toggle.textContent, 'Источники · 2 ▾');
  assert.equal(bot.querySelector('.sources').hidden, true);
  toggle.click();
  assert.equal(toggle.textContent, 'Источники · 2 ▴');
  assert.equal(toggle.getAttribute('aria-expanded'), 'true');
  assert.equal(bot.querySelector('.sources a').textContent, '[1] VPN');
  // the list repeats the numbers used in the answer text, not its own order
  assert.equal(bot.querySelectorAll('.sources a')[1].textContent, '[2, 4] Сеть');
  assert.equal(bot.querySelector('.snip').textContent, '«Установите клиент»');
  assert.equal(bot.querySelectorAll('.fb button').length, 2);
  assert.equal(w.$('.char').dataset.mood, 'happy');
  assert.equal(w.$('.send').disabled, false);
  await tick(1600);
  assert.equal(w.$('.char').dataset.mood, 'idle');
});

test('empty and failed answers', async () => {
  const w = await mount();
  await w.open();
  await w.send('пусто');
  w.lastStream().view({finished: true, mood: 'idle'});
  assert.equal(w.$$('.msg.bot .text')[0].textContent, 'Ответ пуст.');

  await w.send('ошибка');
  w.lastStream().view({finished: true, error: 'AI-сервис временно недоступен.', mood: 'sad'});
  const bot = w.$$('.msg.bot')[1];
  assert.equal(bot.querySelector('.err').textContent, 'AI-сервис временно недоступен.');
  assert.equal(bot.querySelector('.text').textContent, '');
  assert.equal(bot.querySelector('.fb'), null, 'no feedback for a failed answer');
  assert.equal(w.$('.char').dataset.mood, 'sad');
});

test('a question asked while the answer streams is sent right after it; a third one is dropped', async () => {
  const w = await mount();
  await w.open();
  await w.send('один');
  await w.send('два');
  await w.send('три');
  assert.equal(w.streams.length, 1);
  assert.equal(w.$('.composer textarea').value, 'три', 'the dropped question stays in the field');
  w.streams[0].view({text: 'ok', finished: true, mood: 'idle'});
  assert.equal(w.streams.length, 2);
  assert.equal(w.streams[1].payload.message, 'два');
});

test('signed out during the stream: the session is checked again and the login shows', async () => {
  let authed = true;
  const w = await mount({handlers: {session: () => ({authenticated: authed})}});
  await w.open();
  await w.send('вопрос');
  authed = false;
  w.lastStream().send({op: 'signed-out'});
  await flush();
  assert.equal(w.$('.send').disabled, false);
  assert.ok(visible(w.$('.login')));
  // other non-view messages only end the busy state
  authed = true;
  await w.open();
  await w.send('ещё');
  w.lastStream().send({op: 'unsupported'});
  assert.equal(w.$('.send').disabled, false);
});

test('feedback: 👍 thanks; 👎 asks why, sends the comment; failures are reported', async () => {
  const answers = [{ok: true}, {ok: true}, {ok: false}, {ok: true}, {ok: false}];
  const w = await mount({handlers: {feedback: () => answers.shift()}});
  await w.open();
  const finished = async id => {
    await w.send(`q-${id}`);
    w.lastStream().view({text: 'a', finished: true, mood: 'idle', messageId: id});
    return w.$$('.msg.bot').at(-1);
  };

  const up = await finished('m1');
  up.querySelector('[aria-label="Ответ помог"]').click();
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'feedback', messageId: 'm1', rating: 'up'});
  assert.equal(up.querySelector('.fb span').textContent, 'Спасибо!');
  assert.ok([...up.querySelectorAll('.fb button')].every(b => b.disabled));
  assert.ok(up.querySelector('.fb button').classList.contains('on'));

  const down = await finished('m2');
  down.querySelector('[aria-label="Ответ не помог"]').click();
  await flush();
  const why = down.querySelector('.fb-why');
  assert.equal(why.hidden, false);
  why.querySelector('textarea').value = '   ';
  submit(why);
  assert.equal(why.hidden, true, 'an empty comment just closes the form');
  why.hidden = false;
  why.querySelector('textarea').value = 'устарело';
  submit(why);
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'feedback', messageId: 'm2', rating: 'down', comment: 'устарело'});
  assert.equal(down.querySelector('.fb span').textContent, 'Не сохранилось, попробуйте ещё раз');
  assert.equal(why.querySelector('button.primary').disabled, false);
  submit(why);
  await flush();
  assert.equal(down.querySelector('.fb span').textContent, 'Спасибо, учтём!');
  assert.equal(why.hidden, true);

  const failed = await finished('m3');
  failed.querySelector('[aria-label="Ответ не помог"]').click();
  await flush();
  assert.equal(failed.querySelector('.fb span').textContent, 'Не сохранилось');
  assert.equal(failed.querySelector('.fb-why').hidden, true);
  // "Пропустить" hides the comment form
  failed.querySelector('.fb-why').hidden = false;
  failed.querySelector('.fb-why button[type=button]').click();
  assert.equal(failed.querySelector('.fb-why').hidden, true);
});

const pendingCard = (id, extra = {}) => ({id, title: 'Заявка на отсутствие', summary: 'Отгул на завтра', rows: [{label: 'С', value: '01.12.2026'}], pending: true, result: '', ...extra});

test('action card: confirm retries after a retryable error, then succeeds with a portal link', async () => {
  const outcomes = [{tone: 'error', text: 'Не удалось связаться с backend.', retryable: true}, {tone: 'success', text: 'Заявка REQ-1 создана', requestId: 'r1'}];
  const w = await mount({handlers: {confirm: () => outcomes.shift()}, env: {portalUrl: (kind, id) => `https://portal/${kind}/${id}`}});
  await w.open();
  await w.send('оформи отгул');
  w.lastStream().view({text: 'Проверьте заявку', finished: true, mood: 'waiting', card: pendingCard('a1')});
  const card = w.$('.card');
  assert.equal(card.querySelector('b').textContent, 'Заявка на отсутствие');
  assert.equal(card.querySelector('.summary').textContent, 'Отгул на завтра');
  assert.equal(card.querySelector('dt').textContent, 'С');
  assert.equal(card.querySelector('dd').textContent, '01.12.2026');
  assert.equal(w.$('.char').dataset.mood, 'waiting');
  // the same card again does not duplicate it
  w.lastStream().view({text: 'Проверьте заявку', finished: true, mood: 'waiting', card: pendingCard('a1')});
  assert.equal(w.$$('.card').length, 1);

  const [ok, no] = card.querySelectorAll('.row button');
  ok.click();
  assert.equal(card.querySelector('.result').textContent, 'Отправляю заявку…');
  assert.equal(ok.disabled, true);
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'confirm', actionId: 'a1'});
  assert.equal(card.querySelector('.result').dataset.tone, 'error');
  assert.equal(ok.disabled, false);
  assert.equal(no.disabled, false);
  ok.click();
  await flush();
  assert.equal(card.querySelector('.result').textContent, 'Заявка REQ-1 создана');
  assert.equal(card.querySelector('.result').dataset.tone, 'success');
  assert.equal(card.querySelector('.portal-link').href, 'https://portal/assistant/r1');
  assert.equal(card.querySelector('.row'), null);
  assert.equal(w.$('.char').dataset.mood, 'happy');
});

test('action card: cancel, no answer from the server, and a card that is already done', async () => {
  const w = await mount({handlers: {cancel: {tone: 'muted', text: 'Отменено. Заявка не отправлена.'}, confirm: undefined}});
  await w.open();
  await w.send('a');
  w.lastStream().view({finished: true, mood: 'waiting', card: pendingCard('a1')});
  w.$('.card .row button:last-child').click();
  assert.equal(w.$('.card .result').textContent, 'Отменяю…');
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'cancel', actionId: 'a1'});
  assert.equal(w.$('.card .result').textContent, 'Отменено. Заявка не отправлена.');
  assert.equal(w.$('.char').dataset.mood, 'idle');

  await w.send('b');
  w.lastStream().view({finished: true, mood: 'waiting', card: pendingCard('a2')});
  const card = w.$$('.card').at(-1);
  card.querySelector('.row button').click();
  await flush();
  assert.equal(card.querySelector('.result').textContent, 'Не удалось связаться с сервером.');
  assert.equal(card.querySelector('.result').dataset.tone, 'error');
  assert.equal(card.querySelector('.portal-link'), null);
  assert.equal(w.$('.char').dataset.mood, 'idle', 'no outcome: not a known tone');

  await w.send('c');
  w.lastStream().view({finished: true, mood: 'idle', card: pendingCard('a3', {pending: false, result: 'Выполнено.', summary: ''})});
  const done = w.$$('.card').at(-1);
  assert.equal(done.querySelector('.result').textContent, 'Выполнено.');
  assert.equal(done.querySelector('.row'), null);
  assert.equal(done.querySelector('.summary'), null);
});

test('forms asked by the assistant: prefilled, validated by the server, sent, closed', async () => {
  const answers = {
    'request-create': [{error: 'Дата в прошлом'}, {request: {id: 'r7', number: 'REQ-7'}}, {request: {id: 'r8', number: 'REQ-8'}}],
    'note-save': [{note: {id: 'n1'}}],
    'event-create': [undefined, {event: {id: 'e1'}}],
    'question-create': [{question: {id: 'q1'}}],
  };
  const w = await mount({
    handlers: Object.fromEntries(Object.keys(answers).map(op => [op, () => answers[op].shift()])),
    env: {portalUrl: (kind, id) => `https://portal/${kind}/${id}`},
  });
  await w.open();
  const formFor = async (form) => {
    await w.send(`форма ${form.type}`);
    w.lastStream().view({text: 'Заполните форму', finished: true, mood: 'idle', form});
    return w.$$('.form-card').at(-1);
  };

  const leave = await formFor({type: 'leave_request', prefill: {kind: 'vacation', date_from: '2026-12-01', comment: 'семья'}});
  assert.equal(leave.querySelector('b').textContent, 'Заявка на отгул или отпуск');
  const f = leave.querySelector('form');
  assert.equal(f.kind.value, 'vacation');
  assert.equal(f.date_from.value, '2026-12-01');
  assert.equal(f.comment.value, 'семья');
  submit(f);
  assert.equal(leave.querySelector('.bar .muted').textContent, 'Отправляю…');
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'request-create', type: 'leave_request', params: {kind: 'vacation', date_from: '2026-12-01', date_to: '2026-12-01', comment: 'семья'}});
  assert.equal(leave.querySelector('.bar .muted').textContent, 'Дата в прошлом');
  assert.equal(leave.querySelector('button.primary').disabled, false);
  submit(f);
  await flush();
  assert.equal(leave.querySelector('.result').textContent, 'Заявка REQ-7 создана, статус: на согласовании.');
  assert.equal(leave.querySelector('.portal-link').href, 'https://portal/assistant/r7');
  assert.equal(leave.querySelector('form'), null);

  const access = await formFor({type: 'access_request', prefill: {system: 'oracle', resource: 'billing'}});
  const a = access.querySelector('form');
  assert.equal(a.system.value, 'jira', 'an unknown option is not forced into a select');
  assert.equal(a.resource.value, 'billing');
  a.justification.value = '  для задачи  ';
  submit(a);
  await flush();
  assert.deepEqual(w.calls.at(-1).params, {system: 'jira', resource: 'billing', access_level: 'read', justification: 'для задачи'});
  assert.match(access.querySelector('.result').textContent, /REQ-8/);

  const note = await formFor({type: 'note', prefill: {title: 'Идея'}});
  submit(note.querySelector('form'));
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'note-save', title: 'Идея', content: ''});
  assert.equal(note.querySelector('.result').textContent, 'Заметка сохранена (вкладка «Заметки»).');
  assert.equal(note.querySelector('.portal-link'), null);

  const event = await formFor({type: 'event', prefill: {title: 'Встреча'}});
  const e = event.querySelector('form');
  assert.match(e.starts_at.value, /^\d{4}-\d{2}-\d{2}T\d{2}:00$/);
  assert.ok(e.ends_at.value > e.starts_at.value);
  submit(e);
  await flush();
  assert.equal(event.querySelector('.bar .muted').textContent, 'Не получилось. Попробуйте ещё раз.');
  submit(e);
  await flush();
  assert.equal(event.querySelector('.result').textContent, 'Событие добавлено в календарь.');

  const mentor = await formFor({type: 'mentor_question', prefill: {question: 'Кто ревьюит?'}});
  submit(mentor.querySelector('form'));
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'question-create', question: 'Кто ревьюит?'});
  assert.equal(mentor.querySelector('.result').textContent, 'Вопрос передан наставнику.');

  const closed = await formFor({type: 'note'});
  closed.querySelector('.bar button:not(.primary)').click();
  assert.equal(closed.textContent, 'Форма закрыта.');
});

test('sources open in the document reader with the snippet highlighted; back returns to the chat', async () => {
  const docs = [
    {doc: {title: 'VPN гайд', url: 'https://wiki.example.com/vpn-full', updated: '2026-01-10', sections: [
      {heading: 'Шаги', text: 'Вступление\nУстановите клиент VPN и войдите\n- пункт'}, {text: 'Без заголовка'},
    ]}},
    {error: 'Документ не найден в базе знаний.'},
    {doc: {title: 'Пустой', url: 'javascript:alert(1)'}},
  ];
  const w = await mount({handlers: {doc: () => docs.shift()}});
  await w.open();
  await w.send('vpn');
  w.lastStream().view({text: 'См. [1]', finished: true, mood: 'idle', messageId: 'm1', sources: [
    {id: 'kb-vpn', title: 'VPN', url: 'https://wiki.example.com/vpn', snippet: 'Установите **клиент** VPN'},
    {id: '', title: 'Внешняя', url: 'https://example.org/ext', snippet: ''},
  ]});
  const links = w.$$('.sources a');
  assert.equal(links[0].href, 'https://wiki.example.com/vpn');
  assert.equal(links[0].target, '_blank');

  links[0].dispatchEvent(new page.window.MouseEvent('click', {bubbles: true, cancelable: true, ctrlKey: true}));
  links[1].click();
  await flush();
  assert.equal(w.calls.filter(c => c.op === 'doc').length, 0, 'modifier click and id-less sources open the wiki page itself');

  links[0].click();
  assert.ok(w.$('.chat').classList.contains('full'), 'the reader opens in the big window');
  assert.equal(w.$('.reader-body').textContent, 'Открываю документ…');
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'doc', id: 'kb-vpn'});
  assert.equal(w.$('.reader').hidden, false);
  assert.equal(w.$('.logwrap').hidden, true);
  assert.equal(w.$('.composer-wrap').hidden, true);
  assert.equal(w.$('.doc-head h3').textContent, 'VPN гайд');
  assert.match(w.$('.doc-head p').textContent, /обновлено 2026-01-10/);
  assert.equal(w.$('.doc h4').textContent, 'Шаги');
  const hit = w.$('.doc .hit');
  assert.equal(hit.textContent, 'Установите клиент VPN и войдите');
  assert.equal(hit.dataset.scrolledIntoView, '1');
  assert.equal(w.$('.open-full').href, `https://wiki.example.com/vpn-full#:~:text=${encodeURIComponent('Установите клиент VPN')}`);
  assert.equal(w.$('.open-full').hidden, false);

  w.$('.back').click();
  assert.equal(w.$('.reader').hidden, true);
  assert.equal(w.$('.logwrap').hidden, false);

  links[0].click();
  await flush();
  assert.equal(w.$('.reader-body .err').textContent, 'Документ не найден в базе знаний.');

  links[0].click();
  await flush();
  assert.equal(w.$('.doc-head h3').textContent, 'Пустой');
  assert.match(w.$('.doc-head p').textContent, /обновлено —/);
  assert.equal(w.$('.open-full').href, `https://wiki.example.com/vpn#:~:text=${encodeURIComponent('Установите клиент VPN')}`, 'falls back to the source url');

  // asking from the reader closes it
  await w.send('ещё вопрос');
  assert.equal(w.$('.reader').hidden, true);
});

test('suggestions: Enter on the empty field asks the shown question from the knowledge base; Tab pastes it', async () => {
  const hints = [{hint: {text: 'Ответ из базы: поставьте клиент', sources: [{id: 'kb-1', title: 'VPN', url: 'https://wiki/vpn', snippet: ''}], message_id: 'h1'}}];
  const w = await mount({handlers: {suggestions: {suggestions: ['Где взять VPN?']}, 'hint-turn': () => hints.shift() || {error: 'нет'}}});
  await w.open();
  const input = w.$('.composer textarea');
  assert.equal(input.placeholder, 'Где взять VPN?');
  key(input, 'Tab');
  assert.equal(input.value, 'Где взять VPN?');
  input.value = '';
  key(input, 'Enter');
  await flush();
  assert.deepEqual(w.calls.find(c => c.op === 'hint-turn'), {op: 'hint-turn', question: 'Где взять VPN?'});
  assert.equal(w.streams.length, 0, 'the model is not asked');
  const bot = w.$('.msg.bot');
  assert.equal(bot.querySelector('.text').textContent, 'Ответ из базы: поставьте клиент');
  assert.equal(bot.querySelector('.sources').hidden, false, 'a hint keeps its link visible');
  assert.ok(bot.querySelector('.fb'));
  w.api.close();

  // no hint for the question: the model answers it instead
  w.$('.clear').click();
  await flush();
  await w.open();
  await w.send('где взять vpn?');
  assert.equal(w.streams.length, 1);
  assert.equal(w.$$('.msg.user').length, 1);
  assert.equal(w.lastStream().payload.message, 'где взять vpn?');
  // suggestions are loaded once
  assert.equal(w.calls.filter(c => c.op === 'suggestions').length, 1);
});

test('keyboard: Enter sends, Shift+Enter does not, Escape closes; keys swallowed by the page are typed by us', async () => {
  const w = await mount();
  await w.open();
  const input = w.$('.composer textarea');
  input.value = 'текст';
  key(input, 'Enter', {shiftKey: true});
  assert.equal(w.streams.length, 0);
  key(input, 'Enter');
  await flush();
  assert.equal(w.streams.length, 1);

  // a page that cancels letters in a capturing listener (YouTube, Jira...)
  const swallow = event => event.preventDefault();
  document.addEventListener('keydown', swallow, true);
  let leaked = 0;
  const spy = () => { leaked += 1; };
  document.addEventListener('keydown', spy);
  input.value = '';
  key(input, 'ы');
  key(input, '!', {ctrlKey: true});
  key(input, 'Shift');
  document.removeEventListener('keydown', swallow, true);
  document.removeEventListener('keydown', spy);
  assert.equal(input.value, 'ы');
  assert.equal(leaked, 0, 'key events do not leave the widget');

  key(w.$('.composer'), 'Escape');
  assert.equal(w.api.isOpen(), false);
  key(w.$('.body'), 'Enter');
  await flush();
  assert.equal(w.api.isOpen(), true);
  key(w.$('.body'), ' ');
  assert.equal(w.api.isOpen(), false);
});

test('drag moves the character and stores the position; a click without moving opens the chat', async () => {
  const w = await mount({stored: {buddyPos: {x: 300, y: 300}}});
  const body = w.$('.body');
  pointer(body, 'pointerdown', {button: 2});
  pointer(body, 'pointermove', {clientX: 400, clientY: 400});
  assert.deepEqual(w.api.position(), {x: 300, y: 300, size: 72}, 'only the main button drags');

  pointer(body, 'pointerdown', {clientX: 10, clientY: 10});
  pointer(body, 'pointermove', {clientX: 12, clientY: 11});
  assert.equal(w.$('.char').classList.contains('dragging'), false, 'a tiny move is still a click');
  pointer(body, 'pointermove', {clientX: 60, clientY: 40});
  assert.ok(w.$('.char').classList.contains('dragging'));
  pointer(body, 'pointerup');
  await flush();
  assert.deepEqual(w.api.position(), {x: 350, y: 330, size: 72});
  const stored = w.data.get('buddyPos');
  assert.ok(Math.abs(stored.fx - 350 / (innerWidth - 72)) < 1e-9);
  assert.equal(w.api.isOpen(), false);

  pointer(body, 'pointerup'); // no drag in progress
  pointer(body, 'pointerdown', {clientX: 5, clientY: 5});
  pointer(body, 'pointerup');
  await flush();
  assert.equal(w.api.isOpen(), true);
});

test('compact / full mode and the resize grip; the size is remembered per mode', async () => {
  const w = await mount({stored: {buddyPos: {x: 900, y: 650}, buddyChatSizes: {compact: {w: 400, h: 500}}}});
  await w.open();
  const chat = w.$('.chat');
  assert.equal(chat.style.width, '400px');
  const mode = w.$('.mode');
  assert.equal(mode.textContent, '⤢');
  assert.equal(w.$('.grip').dataset.corner, 'tl', 'opens up and to the left of a character in the bottom right');

  const grip = w.$('.grip');
  pointer(grip, 'pointermove', {clientX: 0, clientY: 0});
  pointer(grip, 'pointerup');
  pointer(grip, 'pointerdown', {button: 1});
  pointer(grip, 'pointerdown', {clientX: 100, clientY: 100});
  pointer(grip, 'pointermove', {clientX: 50, clientY: 60});
  pointer(grip, 'pointerup');
  await flush();
  assert.equal(w.data.get('buddyChatSizes').compact.w, 450);

  mode.click();
  await flush();
  assert.ok(chat.classList.contains('full'));
  assert.equal(mode.textContent, '⤡');
  assert.equal(w.data.get('buddyChatMode'), 'full');
  assert.equal(w.$('.tabs').hidden, false);
  assert.equal(w.$('.scrim').hidden, false);
  assert.equal(w.$('.grip').dataset.corner, 'br');
  pointer(grip, 'pointerdown', {clientX: 100, clientY: 100});
  pointer(grip, 'pointermove', {clientX: 110, clientY: 120});
  pointer(grip, 'pointerup');
  await flush();
  assert.ok(w.data.get('buddyChatSizes').full);

  w.$('.scrim').click();
  assert.equal(w.api.isOpen(), false);
  await w.open();
  assert.equal(chat.classList.contains('full'), false, 'opened again: always the small window first');

  // a character at the top left opens the window below him
  w.data.set('buddyPos', {x: 10, y: 10});
  await w.api.syncPosition();
  assert.equal(w.$('.grip').dataset.corner, 'br');
});

test('clear starts a new dialog; logout signs out; × hides the character', async () => {
  let authed = true;
  let hidden = 0;
  const w = await mount({
    handlers: {session: () => ({authenticated: authed}), 'new-dialog': {ok: true}, logout: () => { authed = false; return {ok: true}; }},
    features: {logout: true},
    env: {onHide: () => { hidden += 1; }},
  });
  await w.open();
  await w.send('вопрос');
  const s = w.lastStream();
  w.$('.clear').click();
  await flush();
  assert.equal(s.aborted, true);
  assert.ok(w.ops().includes('new-dialog'));
  assert.equal(w.$('.log').children.length, 0);
  assert.equal(w.$('.send').disabled, false);

  assert.equal(w.$('.out').hidden, false);
  await w.send('ещё');
  w.$('.out').click();
  await flush();
  assert.ok(w.ops().includes('logout'));
  assert.equal(w.lastStream().aborted, true);
  assert.equal(w.$('.log').children.length, 0);
  assert.ok(visible(w.$('.login')));

  w.$('.hide').click();
  assert.equal(hidden, 1);
  assert.equal(w.api.isShown(), false);
});

test('without the extension the site shows a sign and the install steps', async () => {
  const w = await mount({env: {installHint: {url: 'https://example.com/install'}}, features: {hide: false}});
  assert.equal(w.$('.hide').hidden, true);
  assert.equal(w.$('.noext').hidden, false);
  await w.open();
  w.$('.ext').click();
  const node = w.$('.msg.install');
  assert.equal(node.querySelector('b').textContent, 'Расширение Бади не установлено');
  assert.equal(node.querySelectorAll('.install-steps li').length, 3);
  assert.equal(node.querySelector('.install-link').href, 'https://example.com/install');
});

test('page context goes along only when the question is about the page, never with a password field', async () => {
  document.body.innerHTML = `<main>${'Регламент деплоя. '.repeat(30)}</main><footer>подвал</footer>`;
  const w = await mount();
  await w.open();
  await w.send('Как дела?');
  assert.equal(w.lastStream().payload.pageContext, null);
  w.lastStream().view({finished: true, mood: 'idle', text: 'ok'});

  await w.send('Что тут написано?');
  const ctx = w.lastStream().payload.pageContext;
  assert.equal(ctx.url, 'https://intranet.example.com/page');
  assert.equal(ctx.title, 'Портал');
  assert.match(ctx.text, /^Регламент деплоя\./);
  assert.ok(!ctx.text.includes('подвал'), 'the main part of the page only');
  assert.equal(w.$$('.msg.user .ctx').at(-1).textContent, '📄 с учётом страницы');
  w.lastStream().view({finished: true, mood: 'idle', text: 'ok'});

  // a work tool page always goes along; a short main falls back to the whole body; long text is cut
  document.title = 'Jira board';
  document.body.innerHTML = `<main>коротко</main><div>${'x'.repeat(6000)}</div>`;
  await w.send('Как взять задачу?');
  const big = w.lastStream().payload.pageContext;
  assert.equal(big.title, 'Jira board');
  assert.equal(big.text.length, 5501);
  assert.ok(big.text.endsWith('…'));
  w.lastStream().view({finished: true, mood: 'idle', text: 'ok'});

  document.title = '';
  document.body.innerHTML = '<form><input type="password"></form>';
  await w.send('а здесь?');
  const secret = w.lastStream().payload.pageContext;
  assert.deepEqual(secret, {url: 'https://intranet.example.com/page'});
});

test('a host without page context never sends the page', async () => {
  const w = await mount({features: {pageContext: false}});
  await w.open();
  await w.send('что тут?');
  assert.equal(w.lastStream().payload.pageContext, null);
  page.select('выделенный длинный текст');
  pointer(document.body, 'mouseup');
  await tick(0);
  assert.equal(w.$('.chip').hidden, true);
});

test('selection: a chip; every 15th selection offers help; after trying once the hint never shows again', async () => {
  const w = await mount({stored: {buddySelCount: 13}});
  const select = async text => { page.select(text); pointer(document.body, 'mouseup'); await tick(0); };

  await select('коротко');
  assert.equal(w.$('.chip').hidden, true, 'short selections are ignored');
  assert.equal(w.data.get('buddySelCount'), 13);

  await select('Этот   регламент\nдействует с 2024 года');
  assert.equal(w.$('.chip').hidden, false);
  assert.equal(w.$('.chip-text').textContent, 'Выделено: «Этот регламент действует с 2024 года»');
  assert.equal(w.data.get('buddySelCount'), 14);
  assert.equal(w.$('.bubble').hidden, true);
  w.$('.chip-x').click();
  assert.equal(w.$('.chip').hidden, true);

  await select(`Длинный выделенный текст ${'слово '.repeat(20)}`);
  assert.equal(w.data.get('buddySelCount'), 15);
  assert.match(w.$('.chip-text').textContent, /…»$/);
  assert.equal(w.$('.bubble').hidden, false);
  assert.equal(w.$('.bubble-text').textContent, 'Спросить меня про выделенный текст?');
  assert.equal(w.$('.char').dataset.mood, 'curious');
  w.$('.bubble-actions button').click();
  await flush();
  assert.equal(w.api.isOpen(), true);
  const s = w.lastStream();
  assert.equal(s.payload.message, 'Это ещё актуально?');
  assert.match(s.payload.pageContext.selection, /^Длинный выделенный текст/);
  assert.equal(w.$('.msg.user .ctx').textContent, '📄 страница + выделенный текст');
  assert.equal(w.data.get('buddySelTried'), true);
  assert.equal(w.$('.chip').hidden, true, 'the selection is used once');

  // the chat is open: the chip updates, no counting
  await select('ещё один длинный кусок текста');
  assert.equal(w.$('.chip').hidden, false);
  assert.equal(w.data.get('buddySelCount'), 15);

  // tried once: never again
  w.api.close();
  w.data.set('buddySelCount', 29);
  await select('ещё один длинный кусок текста');
  assert.equal(w.data.get('buddySelCount'), 29);
  assert.equal(w.$('.bubble').hidden, true);

  // a mouseup inside the widget is not a page selection
  const before = w.$('.chip-text').textContent;
  page.select('выделено внутри окна Бади');
  pointer(w.$('.body'), 'mouseup');
  await tick(0);
  assert.equal(w.$('.chip-text').textContent, before);
});

test('greeting once a day: signed in it offers the summary of the day', async () => {
  const w = await mount({stored: {buddyGreeting: '2000-01-01'}});
  await tick(1200);
  assert.equal(w.data.get('buddyGreeting'), new Date().toISOString().slice(0, 10));
  assert.equal(w.$('.bubble').hidden, false);
  assert.equal(w.$('.bubble-text').textContent, 'Привет, Аня! У меня есть сводка на сегодня.');
  assert.equal(w.$('.char').dataset.mood, 'waving');
  const button = w.$('.bubble-actions button');
  assert.equal(button.textContent, 'Показать');
  button.click();
  await flush();
  assert.equal(w.$('.bubble').hidden, true);
  assert.match(w.lastStream().payload.message, /краткую сводку на сегодня/);
});

test('greeting signed out offers to sign in; the bubble hides itself after a while', async () => {
  const w = await mount({stored: {buddyGreeting: undefined}, handlers: {session: {authenticated: false}}});
  await tick(1200);
  assert.equal(w.$('.bubble-text').textContent, 'Привет! Я Бади, помогу с онбордингом. Войди, и начнём.');
  assert.equal(w.$('.bubble-actions button').textContent, 'Войти');
  // a passing remark is skipped while another bubble is shown
  await w.api.say('мимо');
  assert.equal(w.$('.bubble-text').textContent, 'Привет! Я Бади, помогу с онбордингом. Войди, и начнём.');
  await tick(14000);
  assert.equal(w.$('.bubble').hidden, true);

  const said = w.api.say('Кликни меня', {ms: 0});
  await flush();
  assert.equal(w.$('.bubble').hidden, false);
  w.$('.bubble').click();
  await flush();
  assert.equal(w.api.isOpen(), true);
  assert.ok(visible(w.$('.login')));
  await said; // resolved when the bubble went away
});

test('page hint: shown once per page in this tab, its question goes with the page', async () => {
  sessionStorage.clear();
  const w = await mount({handlers: {hint: {hint: {text: 'Это дашборды метрик. Подсказать?', question: 'Что тут смотреть?'}}}});
  await tick(1200);
  assert.deepEqual(w.calls.find(c => c.op === 'hint'), {op: 'hint', title: 'Портал', url: 'https://intranet.example.com/page'});
  assert.equal(w.$('.bubble-text').textContent, 'Это дашборды метрик. Подсказать?');
  assert.equal(sessionStorage.getItem('aiBuddyHint:/page'), '1');
  w.$('.bubble-actions button').click();
  await flush();
  const s = w.lastStream();
  assert.equal(s.payload.message, 'Что тут смотреть?');
  assert.equal(s.payload.pageContext.url, 'https://intranet.example.com/page');
  w.api.remove();

  const again = await mount({handlers: {hint: {hint: {text: 'Снова', question: null}}}});
  await tick(1200);
  assert.equal(again.$('.bubble').hidden, true, 'already shown on this page');
  again.api.remove();

  sessionStorage.clear();
  const noQuestion = await mount({handlers: {hint: {hint: {text: 'Выдели текст', question: null}}}});
  await tick(1200);
  assert.equal(noQuestion.$('.bubble-actions').children.length, 0);
  noQuestion.$('.bubble-x').click();
  assert.equal(noQuestion.$('.bubble').hidden, true);
  noQuestion.api.remove();

  const noContext = await mount({handlers: {hint: {hint: {text: 'x'}}}, features: {pageContext: false}});
  await tick(1200);
  assert.ok(!noContext.ops().includes('hint'));
});

test('request status changes are told in a queued bubble, after the chat is closed', async () => {
  const changes = [
    [{number: 'REQ-1', status: 'approved', text: 'Заявка REQ-1: согласована'}],
    [{number: 'REQ-2', status: 'rejected', text: 'Заявка REQ-2: отклонена'}],
    [],
  ];
  const w = await mount({handlers: {requests: () => ({changes: changes.shift() || []})}});
  assert.ok(!w.ops().includes('requests'), 'not polled before the character is on the page');
  await tick(20000);
  assert.equal(w.$('.bubble').hidden, false);
  assert.equal(w.$('.bubble-text').textContent, '🎉 Заявка REQ-1: согласована');
  assert.equal(w.$('.char').dataset.mood, 'happy');
  await w.open();
  assert.equal(w.$('.bubble').hidden, true, 'opening the chat hides the bubble');
  await tick(20000);
  assert.equal(w.ops().filter(op => op === 'requests').length, 2);
  assert.equal(w.$('.bubble').hidden, true, 'waits while the chat is open');
  w.api.close();
  await tick(400);
  assert.equal(w.$('.bubble').hidden, false);
  assert.equal(w.$('.bubble-text').textContent, 'Заявка REQ-2: отклонена');
  assert.equal(w.$('.char').dataset.mood, 'sad');
  w.$('.bubble').click();
  await flush();
  assert.equal(w.api.isOpen(), true);
});

test('requests are not polled when the host has no requests; the API ask() opens the chat and asks', async () => {
  const w = await mount({features: {requests: false}, show: false});
  assert.ok(!w.ops().includes('requests'));
  assert.equal(w.api.isShown(), false);
  w.api.show();
  await w.api.ask('Что делать дальше?');
  await flush();
  assert.equal(w.api.isOpen(), true);
  assert.equal(w.lastStream().payload.message, 'Что делать дальше?');
  w.api.toggle();
  assert.equal(w.api.isOpen(), false);
  // an empty question or a signed out user asks nothing
  await w.api.ask('');
  assert.equal(w.streams.length, 1);
  await w.api.refresh();
});

test('the placeholder rotates between suggestions while the chat is open and the field empty', async () => {
  const w = await mount({handlers: {suggestions: {suggestions: ['A?', 'B?', 'C?']}}});
  await w.open();
  const input = w.$('.composer textarea');
  const seen = new Set([input.placeholder]);
  for (let i = 0; i < 10; i += 1) await tick(9000);
  for (let i = 0; i < 10; i += 1) { w.api.close(); w.api.open(); await flush(); seen.add(input.placeholder); }
  assert.ok(seen.size >= 2, `placeholders seen: ${[...seen]}`);
  for (const p of seen) assert.ok(['A?', 'B?', 'C?'].includes(p));

  // the "↓" button and the log scroll handler
  w.$('.log').dispatchEvent(new page.window.Event('scroll'));
  w.$('.to-bottom').click();
  assert.equal(w.$('.to-bottom').hidden, true);
});

test('the storage failing at start still shows the small window', async () => {
  const w = await mountWidget({env: {storage: {get: async () => { throw new Error('denied'); }, set: async () => {}}}});
  current = w;
  await w.open();
  assert.equal(w.$('.mode').textContent, '⤢');
  assert.ok(w.$('.chat').style.width);
});

test('mentor question card: «Редактировать» cancels the proposal and opens the question in a form', async () => {
  const w = await mount({handlers: {cancel: {tone: 'muted', text: 'Отменено.'}, 'question-create': {question: {id: 'q1'}}}});
  await w.open();
  await w.send('можно ли привести собаку?');
  w.lastStream().view({finished: true, mood: 'waiting',
    card: pendingCard('a3', {title: 'Вопрос наставнику', type: 'mentor_question', edit: {question: 'Можно ли собаку?'}})});
  assert.deepEqual([...w.$$('.card .row button')].map(b => b.textContent), ['Подтвердить', 'Редактировать', 'Отменить']);
  w.$$('.card .row button')[1].click();
  await flush();
  assert.deepEqual(w.calls.find(c => c.op === 'cancel'), {op: 'cancel', actionId: 'a3'});
  const form = w.$('.form-card form');
  assert.equal(form.querySelector('textarea').value, 'Можно ли собаку?');
  form.querySelector('textarea').value = 'Можно ли приходить в офис с собакой?';
  submit(form);
  await flush();
  assert.deepEqual(w.calls.at(-1), {op: 'question-create', question: 'Можно ли приходить в офис с собакой?'});
  assert.equal(w.$('.form-card .result').textContent, 'Вопрос передан наставнику.');
});

test('cards of other actions have no «Редактировать»', async () => {
  const w = await mount({});
  await w.open();
  await w.send('оформи отгул');
  w.lastStream().view({finished: true, mood: 'waiting', card: pendingCard('a4')});
  assert.deepEqual([...w.$$('.card .row button')].map(b => b.textContent), ['Подтвердить', 'Отменить']);
});
