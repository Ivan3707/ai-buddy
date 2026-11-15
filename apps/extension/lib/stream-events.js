// Pure state machine for one assistant message built from the SSE events of
// POST /api/chat/stream and /api/chat/brief/stream (contracts/backend.openapi.json,
// BackendStreamEvent). The UI renders whatever state this reducer returns.

export const STATUS_LABELS = Object.freeze({
  retrieving: 'Ищу в базе знаний…',
  generating: 'Пишу ответ…',
  preparing_action: 'Готовлю заявку…',
});

export const ERROR_LABELS = Object.freeze({
  llm_unavailable: 'AI-сервис временно недоступен.',
  timeout: 'AI не ответил вовремя.',
  rate_limited: 'Слишком много запросов. Подождите немного.',
  internal: 'Внутренняя ошибка сервиса.',
  network: 'Нет связи с backend.',
  interrupted: 'Соединение прервалось до конца ответа.',
});

export function createAssistantState() {
  return {
    conversationId: null,
    messageId: null,
    text: '',
    stage: null,
    sources: [],
    action: null,
    form: null,
    finished: false,
    finishReason: null,
    mode: null,
    latencyMs: null,
    error: null,
  };
}

const isObject = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const isString = value => typeof value === 'string';

const FORM_TYPES = ['leave_request', 'access_request', 'note', 'event', 'mentor_question'];

// The assistant asks the chat to draw a form (instead of asking for details in text). Only known types and string values pass.
export function normalizeForm(value) {
  if (!isObject(value) || !FORM_TYPES.includes(value.type)) return null;
  const prefill = {};
  if (isObject(value.prefill)) {
    for (const [key, item] of Object.entries(value.prefill)) if (isString(item)) prefill[key] = item.slice(0, 8000);
  }
  return {type: value.type, prefill};
}

export function parseEventData(raw) {
  try {
    const value = JSON.parse(raw);
    return isObject(value) ? value : null;
  } catch {
    return null;
  }
}

export function normalizeSources(value) {
  if (!Array.isArray(value)) return [];
  return value
    .filter(source => isObject(source) && isString(source.id) && isString(source.title) && isString(source.url))
    .map(source => ({
      id: source.id,
      title: source.title,
      url: source.url,
      ...(isString(source.snippet) && source.snippet ? {snippet: source.snippet} : {}),
      ...(Array.isArray(source.refs) && source.refs.every(Number.isInteger) && source.refs.length ? {refs: source.refs} : {}),
    }));
}

export function normalizeAction(value) {
  if (!isObject(value) || !isString(value.id) || !isString(value.type) || !isString(value.summary)) return null;
  return {
    id: value.id,
    type: value.type,
    summary: value.summary,
    status: isString(value.status) ? value.status : 'pending',
    params: isObject(value.params) ? value.params : {},
    ...(isString(value.error) ? {error: value.error} : {}),
  };
}

export function errorFrom(code, message) {
  const known = isString(code) && Object.hasOwn(ERROR_LABELS, code);
  return {
    code: known ? code : 'internal',
    // Known codes get a Russian label; the backend message is kept as a fallback only.
    message: known ? ERROR_LABELS[code] : (isString(message) && message ? message : ERROR_LABELS.internal),
  };
}

// event: {type, data} where data is the raw JSON string from the SSE frame.
export function applyStreamEvent(state, event) {
  if (state.finished) return state; // exactly one done|error terminates the stream
  const data = parseEventData(event.data);
  if (!data) return state;
  switch (event.type) {
    case 'meta':
      return {
        ...state,
        conversationId: isString(data.conversation_id) ? data.conversation_id : state.conversationId,
        messageId: isString(data.message_id) ? data.message_id : state.messageId,
      };
    case 'status':
      return Object.hasOwn(STATUS_LABELS, data.stage) ? {...state, stage: data.stage} : state;
    case 'delta':
      return isString(data.text) ? {...state, text: state.text + data.text} : state;
    case 'sources':
      return {...state, sources: normalizeSources(data.sources)};
    case 'action': {
      const action = normalizeAction(data.action);
      return action ? {...state, action} : state;
    }
    case 'form': {
      const form = normalizeForm(data.form);
      return form ? {...state, form} : state;
    }
    case 'done':
      return {
        ...state,
        stage: null,
        finished: true,
        finishReason: isString(data.finish_reason) ? data.finish_reason : 'answered',
        mode: isString(data.mode) ? data.mode : null,
        latencyMs: Number.isInteger(data.latency_ms) ? data.latency_ms : null,
      };
    case 'error':
      return {...state, stage: null, finished: true, error: errorFrom(data.code, data.message)};
    default:
      return state; // forward compatible: unknown events are ignored
  }
}

// Called when the byte stream ends (or fails) without a terminal event.
export function finalizeStream(state, code = 'interrupted') {
  if (state.finished) return state;
  return {...state, stage: null, finished: true, error: errorFrom(code)};
}

export function statusLabel(state) {
  if (state.finished) return '';
  return STATUS_LABELS[state.stage] || STATUS_LABELS.retrieving;
}

// Maps a thrown request failure (not an SSE error event) to an error code.
export function failureCode(error) {
  if (error?.name === 'StreamIdleTimeoutError' || error?.name === 'TimeoutError') return 'timeout';
  if (typeof error?.status === 'number') {
    if (error.status === 429) return 'rate_limited';
    if (error.status === 502 || error.status === 503 || error.status === 504) return 'llm_unavailable';
    return 'internal';
  }
  return 'network'; // fetch() rejects with TypeError when the backend is unreachable
}

// Converts the legacy POST /api/chat response into the same event sequence, so the
// UI has a single rendering path.
export function legacyResponseToEvents(response) {
  const events = [];
  const sources = normalizeSources(response?.sources);
  if (sources.length) events.push({type: 'sources', data: JSON.stringify({sources})});
  events.push({type: 'delta', data: JSON.stringify({text: isString(response?.answer) ? response.answer : ''})});
  events.push({type: 'done', data: JSON.stringify({finish_reason: 'answered', mode: response?.mode ?? 'mock'})});
  return events;
}
