#!/usr/bin/env node
// Dev-only mock of the AI-buddy backend for manual testing of the side panel
// (streaming, actions, feedback, brief) before the real Go endpoints exist.
// Usage:
//   node apps/extension/dev/mock-server.mjs [--port 18090] [--legacy]
//   BACKEND_ORIGIN=http://127.0.0.1:18090 npm run build -w @ai-buddy/extension
// --legacy answers 404 on the streaming/brief/action routes like the current backend.
// Any email/password logs in. Synthetic data only; not part of the coverage threshold.
import { createServer } from 'node:http';
import { randomUUID } from 'node:crypto';

const args = process.argv.slice(2);
const port = Number(args[args.indexOf('--port') + 1]) || 18090;
const legacy = args.includes('--legacy');
const token = 'mock-token';
const user = {id: '00000000-0000-4000-8000-000000000001', email: 'intern@example.com', role: 'intern', profession: 'backend', created_at: new Date().toISOString()};
const actions = new Map();
let requestCounter = 0;

const sources = {
  leave: {id: 'kb-leave', title: 'Отгулы и отпуск (демо)', url: 'https://kb.example.com/leave', snippet: 'Отгул оформляется заявкой не позднее чем за 3 рабочих дня.'},
  access: {id: 'kb-access', title: 'Доступы к системам (демо)', url: 'https://kb.example.com/access', snippet: 'Доступ в Grafana на чтение выдаётся по заявке с обоснованием.'},
};

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

function json(res, status, body) {
  res.writeHead(status, {'Content-Type': 'application/json'});
  res.end(body === undefined ? '' : JSON.stringify(body));
}

async function readBody(req) {
  let raw = '';
  for await (const chunk of req) raw += chunk;
  return raw ? JSON.parse(raw) : {};
}

function pendingAction(type, params, summary) {
  const action = {id: randomUUID(), type, params, summary, status: 'pending', created_at: new Date().toISOString()};
  actions.set(action.id, action);
  return action;
}

function scenario(message, pageContext) {
  const text = message.toLowerCase();
  if (text.includes('ошибк')) return {error: {code: 'llm_unavailable', message: 'LLM provider is down'}};
  if (text.includes('отгул') || text.includes('отпуск')) {
    return {
      answer: 'Отгул оформляется заявкой в системе отсутствий не позднее чем за 3 рабочих дня. Я подготовил заявку — проверьте даты и подтвердите.',
      sources: [sources.leave],
      action: pendingAction('leave_request', {kind: 'day_off', date_from: '2026-11-20', date_to: '2026-11-20', comment: 'Личные дела'}, 'Отгул на 20.11.2026'),
    };
  }
  if (text.includes('доступ') || text.includes('grafana')) {
    return {
      answer: 'Доступ в Grafana на чтение выдаётся по заявке. После выдачи откройте дашборды сервиса и найдите два графика latency.',
      sources: [sources.access],
      action: pendingAction('access_request', {system: 'grafana', resource: 'payments-dashboards', access_level: 'read', justification: 'Задача онбординга: изучить дашборды'}, 'Доступ на чтение к Grafana'),
    };
  }
  if (text.includes('страниц')) {
    const where = pageContext ? `Вы на странице «${pageContext.title || pageContext.url}».` : 'Контекст страницы не передан.';
    const selected = pageContext?.selection ? ` Выделено: «${pageContext.selection.slice(0, 80)}».` : '';
    return {answer: `${where}${selected} (демо-ответ мок-сервера)`, sources: []};
  }
  return {
    answer: 'В демо-базе знаний нет документа по этому вопросу. Могу передать вопрос наставнику.',
    sources: [],
    finishReason: 'no_answer',
    action: pendingAction('mentor_question', {question: message, context: 'Документ в базе знаний не найден'}, 'Передать вопрос наставнику'),
  };
}

async function stream(res, plan) {
  res.writeHead(200, {'Content-Type': 'text/event-stream; charset=utf-8', 'Cache-Control': 'no-cache'});
  const send = (type, data) => res.write(`event: ${type}\ndata: ${JSON.stringify(data)}\n\n`);
  const started = Date.now();
  send('meta', {conversation_id: plan.conversationId, message_id: randomUUID()});
  send('status', {stage: 'retrieving'});
  await sleep(500);
  if (plan.error) {
    send('error', plan.error);
    res.end();
    return;
  }
  send('status', {stage: 'generating'});
  for (const word of plan.answer.split(/(?<= )/)) {
    send('delta', {text: word});
    await sleep(40);
  }
  if (plan.sources.length) send('sources', {sources: plan.sources});
  if (plan.action) {
    send('status', {stage: 'preparing_action'});
    await sleep(400);
    send('action', {action: plan.action});
  }
  send('done', {finish_reason: plan.finishReason || (plan.action ? 'action_proposed' : 'answered'), mode: 'mock', latency_ms: Date.now() - started});
  res.end();
}

createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const route = `${req.method} ${url.pathname}`;
  const authorized = req.headers.authorization === `Bearer ${token}`;
  try {
    if (route === 'POST /api/auth/login') return json(res, 200, {access_token: token, user});
    if (!authorized) return json(res, 401, {error: 'unauthorized'});
    if (route === 'GET /api/auth/me') return json(res, 200, user);
    if (route === 'POST /api/chat') {
      const body = await readBody(req);
      const plan = scenario(body.message || '');
      return json(res, 200, {answer: plan.answer || 'Ошибка мок-сервера', sources: plan.sources || [], mode: 'mock'});
    }
    if (legacy) {
      res.writeHead(404, {'Content-Type': 'text/plain; charset=utf-8'});
      return res.end('404 page not found\n');
    }
    if (route === 'POST /api/chat/stream') {
      const body = await readBody(req);
      return stream(res, {...scenario(body.message || '', body.page_context), conversationId: body.conversation_id || randomUUID()});
    }
    if (route === 'POST /api/chat/brief/stream') {
      return stream(res, {
        conversationId: randomUUID(),
        answer: 'Доброе утро! Вы на 2-й неделе. Следующий шаг: получить доступ в Grafana и найти два дашборда вашего сервиса. Спросите «как получить доступ в Grafana?».',
        sources: [sources.access],
      });
    }
    const actionRoute = /^POST \/api\/actions\/([^/]+)\/(confirm|cancel)$/.exec(route);
    if (actionRoute) {
      const action = actions.get(decodeURIComponent(actionRoute[1]));
      if (!action) return json(res, 404, {error: 'action not found'});
      if (actionRoute[2] === 'cancel') {
        if (action.status === 'executed') return json(res, 409, {error: 'already executed'});
        action.status = 'cancelled';
        return json(res, 200, action);
      }
      if (action.status === 'cancelled') return json(res, 409, {error: 'action cancelled'});
      action.status = 'executed';
      if (action.type === 'mentor_question') {
        return json(res, 200, {action, mentor_question: {id: randomUUID(), intern_id: user.id, question: action.params.question, status: 'open', created_at: new Date().toISOString()}});
      }
      requestCounter += 1;
      const prefix = action.type === 'leave_request' ? 'ABS' : 'ACC';
      return json(res, 200, {
        action,
        request: {
          id: randomUUID(), number: `${prefix}-${String(requestCounter).padStart(6, '0')}`, type: action.type,
          status: 'pending_approval', approver: 'Наставник (демо)', params: action.params,
          created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
        },
      });
    }
    if (/^POST \/api\/chat\/messages\/[^/]+\/feedback$/.test(route)) {
      res.writeHead(204);
      return res.end();
    }
    return json(res, 404, {error: 'not found'});
  } catch (error) {
    console.error(error);
    return json(res, 500, {error: 'mock server error'});
  }
}).listen(port, '127.0.0.1', () => {
  console.log(`AI-buddy mock backend on http://127.0.0.1:${port}${legacy ? ' (legacy mode: no streaming routes)' : ''}`);
  console.log('Login with any email/password.');
});
