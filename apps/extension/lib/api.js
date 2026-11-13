// Backend calls of the side panel that are not (yet) in packages/client:
// SSE streaming chat with a fallback to the legacy POST /api/chat.
import { legacyResponseToEvents } from './stream-events.js';
import { readSseStream } from './sse.js';

export class HttpError extends Error {
  constructor(status, message) {
    super(message);
    this.name = 'HttpError';
    this.status = status;
  }
}

export function notifyUnauthorized() {
  if (typeof globalThis.dispatchEvent === 'function') {
    globalThis.dispatchEvent(new Event('ai-buddy:unauthorized'));
  }
}

const isJson = contentType => /\bapplication\/(?:[\w.+-]+\+)?json\b/i.test(contentType || '');

// What to do when POST /api/chat/stream did not return 200.
// - The route is missing in the current backend: Go's ServeMux answers 404 with a
//   text/plain body (or 405 if only other methods exist) -> use the legacy endpoint.
// - The contract also uses 404 (JSON {error}) for "conversation not found": if we sent
//   a conversation_id, drop it and start a new conversation.
export function decideStreamFallback({status, contentType = '', sentConversationId = false}) {
  if (status === 401) return 'unauthorized';
  if (status === 405 || status === 501) return 'legacy';
  if (status === 404) {
    if (isJson(contentType) && sentConversationId) return 'reset-conversation';
    return 'legacy';
  }
  return 'error';
}

async function errorMessage(response) {
  try {
    const payload = await response.json();
    if (typeof payload?.error === 'string') return payload.error;
  } catch { /* empty or non-JSON body */ }
  return `Backend: HTTP ${response.status}`;
}

// JSON POST with bearer auth. Resolves with the parsed body (undefined for 204).
export async function postJson({baseUrl, path, token, body, fetchImpl = globalThis.fetch, timeoutMs = 15000}) {
  const response = await fetchImpl(`${baseUrl}${path}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : {'Content-Type': 'application/json'}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(timeoutMs),
  });
  if (!response.ok) {
    if (response.status === 401) notifyUnauthorized();
    throw new HttpError(response.status, await errorMessage(response));
  }
  if (response.status === 204) return undefined;
  const raw = await response.text();
  return raw ? JSON.parse(raw) : undefined;
}

export function confirmAction(options, actionId) {
  return postJson({...options, path: `/api/actions/${encodeURIComponent(actionId)}/confirm`});
}

export function cancelAction(options, actionId) {
  return postJson({...options, path: `/api/actions/${encodeURIComponent(actionId)}/cancel`});
}

function postStream(fetchImpl, url, token, body, signal) {
  return fetchImpl(url, {
    method: 'POST',
    headers: {
      Accept: 'text/event-stream',
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : {'Content-Type': 'application/json'}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
}

async function consume(response, onEvent, idleTimeoutMs) {
  const contentType = response.headers.get('content-type') || '';
  if (!response.body || !/text\/event-stream/i.test(contentType)) {
    throw new HttpError(response.status, 'Backend returned a non-stream response');
  }
  await readSseStream(response.body, onEvent, {idleTimeoutMs});
}

/**
 * Streams an answer. Resolves with {transport: 'stream' | 'legacy', conversationReset}.
 * onEvent receives {type, data} SSE events (legacy answers are converted to events).
 * Throws HttpError for HTTP failures and StreamIdleTimeoutError for a stalled stream.
 */
export async function streamChat({
  baseUrl, token, request, onEvent, legacyChat,
  fetchImpl = globalThis.fetch, signal, idleTimeoutMs = 60000, preferLegacy = false,
}) {
  let body = request;
  let conversationReset = false;
  if (!preferLegacy) {
    for (let attempt = 0; attempt < 2; attempt += 1) {
      const response = await postStream(fetchImpl, `${baseUrl}/api/chat/stream`, token, body, signal);
      if (response.ok) {
        await consume(response, onEvent, idleTimeoutMs);
        return {transport: 'stream', conversationReset};
      }
      const decision = decideStreamFallback({
        status: response.status,
        contentType: response.headers.get('content-type'),
        sentConversationId: Boolean(body.conversation_id),
      });
      if (decision === 'reset-conversation') {
        const {conversation_id: _dropped, ...rest} = body;
        body = rest;
        conversationReset = true;
        continue;
      }
      if (decision === 'legacy') break;
      if (decision === 'unauthorized') notifyUnauthorized();
      throw new HttpError(response.status, await errorMessage(response));
    }
  }
  const legacy = await legacyChat(body.message);
  for (const event of legacyResponseToEvents(legacy)) onEvent(event);
  return {transport: 'legacy', conversationReset};
}
