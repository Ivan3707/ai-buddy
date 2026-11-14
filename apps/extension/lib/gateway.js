// Backend gateway of the Бади widget (buddy/widget.js). The widget is a pure view: it sends commands and gets
// ready-to-render view models back. The same gateway runs in two places:
// - the extension's background worker (token in chrome.storage.session, never visible to web pages);
// - the AI-buddy web app (token of the site session).
// Only the session (lib/auth.js), the key-value store and the client functions differ (injected).
import { describeActionError, describeResult } from './actions.js';
import { cancelAction, confirmAction, HttpError, sendFeedback, streamBrief, streamChat } from './api.js';
import { displayName, pageHint, requestStatusChanges, toView } from './buddy.js';
import { applyStreamEvent, createAssistantState, failureCode, finalizeStream } from './stream-events.js';

const DOC_ID = /^[A-Za-z0-9_.-]{1,128}$/;

/**
 * @param {object} o
 * @param {string} o.baseUrl backend origin ('' = same origin)
 * @param {object} o.auth session core from lib/auth.js (token(), login(), logout())
 * @param {{onLogin?(): void, onLogout?(): void}} [o.hooks] side effects of an explicit sign-in / sign-out
 * @param {{get(key: string): Promise<any>, set(key: string, value: any): Promise<void>, remove(key: string): Promise<void>}} o.store
 * @param {{getMe(baseUrl: string, token: string): Promise<any>}} o.client
 * @param {typeof fetch} [o.fetchImpl]
 */
export function createGateway({baseUrl, auth, hooks = {}, store, client, fetchImpl = globalThis.fetch}) {
  const options = async () => ({baseUrl, token: await auth.token(), fetchImpl});

  // One retry after 401: another context may have rotated the token, or it expired between the check and the call.
  async function authed(send) {
    const token = await auth.token();
    let response = await send(token);
    if (response.status === 401) {
      const next = await auth.token({force: true, stale: token});
      if (next && next !== token) response = await send(next);
    }
    return response;
  }

  async function getJson(path) {
    const response = await authed(token => fetchImpl(`${baseUrl}${path}`, {
      headers: {Authorization: `Bearer ${token}`}, signal: AbortSignal.timeout(15000),
    }));
    if (!response.ok) throw new HttpError(response.status, `HTTP ${response.status}`);
    return response.json();
  }

  // Writes (notes, request form): the error text comes from the backend's JSON {error}.
  async function sendJson(method, path, body) {
    const response = await authed(token => fetchImpl(`${baseUrl}${path}`, {
      method, headers: {Authorization: `Bearer ${token}`, 'Content-Type': 'application/json'},
      body: JSON.stringify(body), signal: AbortSignal.timeout(15000),
    }));
    let data = null;
    try { data = await response.json(); } catch { /* empty body */ }
    if (!response.ok) throw new HttpError(response.status, data?.error || `HTTP ${response.status}`);
    return data;
  }
  const failure = (error, fallback) => ({error: error?.status === 401 ? 'Сессия истекла. Обновите страницу.' : error?.message && error.status ? error.message : fallback});

  // Every request of the user: the assistant's (leave and access, from the chat or the form) and the corporate
  // service's access tickets (mock service). One shape; statuses are named like the assistant's.
  const FROM_CORPORATE = {open: 'pending_approval', approved: 'approved', denied: 'rejected'};
  async function allRequests() {
    const [own, corporate] = await Promise.allSettled([getJson('/api/requests'), getJson('/api/corporate/access-requests')]);
    if (own.status === 'rejected' && corporate.status === 'rejected') throw own.reason;
    const items = [];
    if (own.status === 'fulfilled') {
      for (const r of own.value) {
        items.push({kind: 'assistant', id: r.id, number: r.number, status: r.status, type: r.type, params: r.params || {},
          approver: r.approver || '', created_at: r.created_at});
      }
    }
    if (corporate.status === 'fulfilled') {
      for (const r of corporate.value) {
        items.push({kind: 'corporate', id: r.id, number: r.ticket_number, status: FROM_CORPORATE[r.status] || 'pending_approval',
          type: 'corporate_access', params: {tool: r.tool}, approver: '', created_at: r.created_at});
      }
    }
    return items.sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
  }

  async function call(message) {
    const token = await auth.token();
    switch (message?.op) {
      case 'session': {
        if (!token) return {authenticated: false};
        try {
          let me;
          try { me = await client.getMe(baseUrl, token); } catch (error) {
            if (error?.status !== 401) throw error;
            const next = await auth.token({force: true, stale: token});
            if (!next || next === token) throw error;
            me = await client.getMe(baseUrl, next);
          }
          return {authenticated: true, user: {id: me.id, name: displayName(me)}}; // the page gets no more than this
        } catch (error) {
          return {authenticated: false, unavailable: error?.status !== 401};
        }
      }
      case 'login': {
        try {
          await auth.login(String(message.email), String(message.password));
          await store.remove('conversationId');
          hooks.onLogin?.();
          return {ok: true};
        } catch (error) {
          return {ok: false, error: error?.status === 401 ? 'Неверный email или пароль.' : 'Backend недоступен.'};
        }
      }
      case 'logout':
        await auth.logout();
        await store.remove('conversationId');
        hooks.onLogout?.();
        return {ok: true};
      case 'confirm':
      case 'cancel': {
        try {
          if (message.op === 'confirm') return describeResult(await confirmAction(await options(), message.actionId));
          await cancelAction(await options(), message.actionId);
          return {tone: 'muted', text: 'Отменено. Заявка не отправлена.'};
        } catch (error) {
          const {text, retryable} = describeActionError(error?.status, message.op);
          return {tone: 'error', text, retryable};
        }
      }
      case 'feedback':
        try { await sendFeedback(await options(), message.messageId, message.rating); return {ok: true}; } catch { return {ok: false}; }
      case 'new-dialog':
        await store.remove('conversationId');
        return {ok: true};
      case 'requests': {
        if (!token) return {changes: []};
        try {
          const {changes, next} = requestStatusChanges(await store.get('buddyRequests'), await allRequests());
          await store.set('buddyRequests', next);
          return {changes};
        } catch { return {changes: []}; }
      }
      case 'hint':
        return {hint: pageHint({title: String(message.title || ''), url: String(message.url || '')})};
      case 'doc': {
        if (!DOC_ID.test(String(message.id || ''))) return {error: 'Документ не найден.'};
        try {
          return {doc: await getJson(`/api/kb/docs/${encodeURIComponent(message.id)}`)};
        } catch (error) {
          return {error: error?.status === 404 ? 'Документ не найден в базе знаний.' : 'Не удалось загрузить документ.'};
        }
      }
      // ---- full mode tabs: notes and requests
      case 'notes':
        try { return {notes: await getJson('/api/notes')}; } catch (error) { return failure(error, 'Не удалось загрузить заметки.'); }
      case 'note-save': {
        const input = {title: String(message.title || '').trim().slice(0, 160), content: String(message.content || '').slice(0, 8000)};
        if (!input.title) return {error: 'Введите заголовок.'};
        try {
          const id = typeof message.id === 'string' ? message.id : '';
          return {note: id ? await sendJson('PUT', `/api/notes/${encodeURIComponent(id)}`, input) : await sendJson('POST', '/api/notes', input)};
        } catch (error) { return failure(error, 'Не удалось сохранить заметку.'); }
      }
      case 'requests-list':
        try { return {requests: await allRequests()}; } catch (error) { return failure(error, 'Не удалось загрузить заявки.'); }
      case 'request-create': {
        const kind = message.type === 'access_request' ? 'access_request' : 'leave_request';
        const params = {};
        for (const [key, value] of Object.entries(message.params || {})) if (typeof value === 'string') params[key] = value.slice(0, 500);
        try { return {request: await sendJson('POST', '/api/requests', {type: kind, params})}; }
        catch (error) { return failure(error, 'Не удалось создать заявку.'); }
      }
      case 'suggestions': // ready-made questions for the placeholder of the empty input
        try {
          const data = await getJson('/api/suggestions');
          return {suggestions: (data.suggestions || []).map(item => String(item.question)).filter(Boolean)};
        } catch { return {suggestions: []}; }
      case 'hint-turn': { // a suggestion answered without the model: excerpt + link, saved into the dialog
        const question = String(message.question || '').trim().slice(0, 300);
        const cid = await store.get('conversationId');
        try {
          const hint = await sendJson('POST', '/api/chat/hint', {question, ...(typeof cid === 'string' && cid ? {conversation_id: cid} : {})});
          if (hint?.conversation_id) await store.set('conversationId', hint.conversation_id);
          return {hint};
        } catch (error) { return failure(error, 'Не удалось получить подсказку.'); }
      }
      case 'questions':
        try { return {questions: await getJson('/api/questions')}; } catch (error) { return failure(error, 'Не удалось загрузить вопросы.'); }
      case 'question-create': {
        const question = String(message.question || '').trim().slice(0, 2000);
        if (!question) return {error: 'Напишите вопрос.'};
        try { return {question: await sendJson('POST', '/api/questions', {question})}; } catch (error) { return failure(error, 'Не удалось отправить вопрос.'); }
      }
      case 'events':
        try { return {events: await getJson('/api/calendar/events')}; } catch (error) { return failure(error, 'Не удалось загрузить календарь.'); }
      case 'event-create': {
        const title = String(message.title || '').trim().slice(0, 160);
        const start = new Date(String(message.starts_at)), end = new Date(String(message.ends_at));
        if (!title) return {error: 'Введите название.'};
        if (!(end > start)) return {error: 'Окончание должно быть позже начала.'};
        try {
          return {event: await sendJson('POST', '/api/calendar/events', {
            title, description: String(message.description || '').slice(0, 2000), starts_at: start.toISOString(), ends_at: end.toISOString(),
          })};
        } catch (error) { return failure(error, 'Не удалось создать событие.'); }
      }
      default:
        return {error: 'unknown op'};
    }
  }

  /** Streams an answer ({op: 'chat', message, pageContext} or {op: 'brief'}) as view models; returns abort(). */
  function stream(payload, send) {
    const controller = new AbortController();
    const {signal} = controller;
    void (async () => {
      let state = createAssistantState();
      send({op: 'view', view: toView(state)});
      const token = await auth.token();
      if (!token) { send({op: 'signed-out'}); return; }
      const onEvent = event => { state = applyStreamEvent(state, event); send({op: 'view', view: toView(state)}); };
      try {
        if (payload.op === 'brief') {
          const result = await streamBrief({baseUrl, token, signal, onEvent, fetchImpl});
          if (result.transport === 'unsupported') { send({op: 'unsupported'}); return; }
        } else {
          const cid = await store.get('conversationId');
          await streamChat({
            baseUrl, token, signal, onEvent, fetchImpl,
            request: {
              message: payload.message,
              ...(typeof cid === 'string' && cid ? {conversation_id: cid} : {}),
              ...(payload.pageContext ? {page_context: payload.pageContext} : {}),
            },
            // The widget needs streaming + cards; without /api/chat/stream the backend counts as unavailable.
            legacyChat: async () => { throw Object.assign(new Error('stream endpoint missing'), {status: 503}); },
          });
        }
        state = finalizeStream(state);
        if (payload.op === 'chat' && state.conversationId) await store.set('conversationId', state.conversationId);
      } catch (error) {
        if (error?.status === 401) { send({op: 'signed-out'}); return; }
        state = finalizeStream(state, signal.aborted ? 'interrupted' : failureCode(error));
      }
      send({op: 'view', view: toView(state)});
    })();
    return () => controller.abort();
  }

  return {call, stream};
}
