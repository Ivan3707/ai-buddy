// Backend gateway of the Бади widget (buddy/widget.js). The widget is a pure view: it sends commands and gets
// ready-to-render view models back. The same gateway runs in two places:
// - the extension's background worker (token in chrome.storage.session, never visible to web pages);
// - the AI-buddy web app (token of the site session).
// Only the token store, the key-value store and the client functions differ (injected).
import { describeActionError, describeResult } from './actions.js';
import { cancelAction, confirmAction, HttpError, sendFeedback, streamBrief, streamChat } from './api.js';
import { displayName, pageHint, requestStatusChanges, toView } from './buddy.js';
import { applyStreamEvent, createAssistantState, failureCode, finalizeStream } from './stream-events.js';

const DOC_ID = /^[A-Za-z0-9_.-]{1,128}$/;

/**
 * @param {object} o
 * @param {string} o.baseUrl backend origin ('' = same origin)
 * @param {{get(): Promise<string>, set?(token: string): Promise<void>}} o.tokens
 * @param {{get(key: string): Promise<any>, set(key: string, value: any): Promise<void>, remove(key: string): Promise<void>}} o.store
 * @param {{getMe(baseUrl: string, token: string): Promise<any>, login?(baseUrl: string, email: string, password: string): Promise<any>}} o.client
 * @param {typeof fetch} [o.fetchImpl]
 */
export function createGateway({baseUrl, tokens, store, client, fetchImpl = globalThis.fetch}) {
  const options = async () => ({baseUrl, token: await tokens.get(), fetchImpl});

  async function getJson(path) {
    const {token} = await options();
    const response = await fetchImpl(`${baseUrl}${path}`, {
      headers: {Authorization: `Bearer ${token}`}, signal: AbortSignal.timeout(15000),
    });
    if (!response.ok) throw new HttpError(response.status, `HTTP ${response.status}`);
    return response.json();
  }

  // Writes (notes, request form): the error text comes from the backend's JSON {error}.
  async function sendJson(method, path, body) {
    const {token} = await options();
    const response = await fetchImpl(`${baseUrl}${path}`, {
      method, headers: {Authorization: `Bearer ${token}`, 'Content-Type': 'application/json'},
      body: JSON.stringify(body), signal: AbortSignal.timeout(15000),
    });
    let data = null;
    try { data = await response.json(); } catch { /* empty body */ }
    if (!response.ok) throw new HttpError(response.status, data?.error || `HTTP ${response.status}`);
    return data;
  }
  const failure = (error, fallback) => ({error: error?.status === 401 ? 'Сессия истекла. Обновите страницу.' : error?.message && error.status ? error.message : fallback});

  async function call(message) {
    const token = await tokens.get();
    switch (message?.op) {
      case 'session': {
        if (!token) return {authenticated: false};
        try {
          const me = await client.getMe(baseUrl, token);
          return {authenticated: true, user: {id: me.id, name: displayName(me)}}; // the page gets no more than this
        } catch (error) {
          return {authenticated: false, unavailable: error?.status !== 401};
        }
      }
      case 'login': {
        if (!client.login || !tokens.set) return {ok: false, error: 'Вход здесь недоступен.'};
        try {
          const result = await client.login(baseUrl, String(message.email), String(message.password));
          await tokens.set(result.access_token);
          await store.remove('conversationId');
          return {ok: true};
        } catch (error) {
          return {ok: false, error: error?.status === 401 ? 'Неверный email или пароль.' : 'Backend недоступен.'};
        }
      }
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
          const {changes, next} = requestStatusChanges(await store.get('buddyRequests'), await getJson('/api/requests'));
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
        try { return {requests: await getJson('/api/requests')}; } catch (error) { return failure(error, 'Не удалось загрузить заявки.'); }
      case 'request-create': {
        const kind = message.type === 'access_request' ? 'access_request' : 'leave_request';
        const params = {};
        for (const [key, value] of Object.entries(message.params || {})) if (typeof value === 'string') params[key] = value.slice(0, 500);
        try { return {request: await sendJson('POST', '/api/requests', {type: kind, params})}; }
        catch (error) { return failure(error, 'Не удалось создать заявку.'); }
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
      const token = await tokens.get();
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
