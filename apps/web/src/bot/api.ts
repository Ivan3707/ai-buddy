// Bot API for the web app: streaming chat (SSE over fetch), actions, requests, hints and the buddy side.
// Contract: contracts/backend.openapi.json. Same-origin '' base: nginx proxies /api to the backend.

export type Source = { id: string; title: string; url: string; snippet?: string };
export type ActionType = 'leave_request' | 'access_request' | 'mentor_question';
export type PendingAction = {
  id: string; type: ActionType; params: Record<string, string>; summary: string;
  status: 'pending' | 'executed' | 'cancelled' | 'failed'; error?: string; created_at: string;
};
export type ServiceRequest = {
  id: string; number: string; type: 'leave_request' | 'access_request';
  status: 'pending_approval' | 'approved' | 'rejected'; approver?: string;
  params: Record<string, string>; created_at: string; updated_at: string;
};
export type MentorQuestion = {
  id: string; intern_id: string; intern_name?: string; question: string; context?: string;
  status: 'open' | 'answered'; answer?: string; created_at: string; answered_at?: string;
};
export type ActionResult = { action: PendingAction; request?: ServiceRequest; mentor_question?: MentorQuestion };
export type ChatMessage = {
  id: string; role: 'user' | 'assistant'; content: string; sources?: Source[]; action?: PendingAction;
  finish_reason?: string; feedback: 'up' | 'down' | null; created_at: string;
};
export type Hint = { id: string; item_id: string; text: string; read_at: string | null; created_at: string };
export type MentorDraft = { draft: string; sources: Source[]; confidence: 'grounded' | 'partial' | 'none'; kb_suggestion?: { title: string; text: string } };
export type ChatMetrics = {
  questions_total: number; answered_by_bot: number; escalated_to_mentor: number; deflection_rate: number | null;
  actions_executed: number; estimated_mentor_minutes_saved: number;
};
export type GapsReport = { total_questions: number; clusters: { label: string; count: number; examples: string[] }[] };
export type KbLintReport = { issues: { kind: string; doc_ids: string[]; detail: string }[] };
export type PageContext = { url: string; title?: string; selection?: string };

export class BotError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export async function request<T>(token: string, path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { Authorization: `Bearer ${token}`, ...(init.body ? { 'Content-Type': 'application/json' } : {}), ...init.headers },
  });
  if (response.status === 401) dispatchEvent(new Event('ai-buddy:unauthorized'));
  if (!response.ok) {
    let message = `Ошибка ${response.status}`;
    try { message = (await response.json()).error || message; } catch { /* not JSON */ }
    throw new BotError(response.status, message);
  }
  return response.status === 204 ? (undefined as T) : response.json();
}

export const post = <T>(token: string, path: string, body?: unknown) =>
  request<T>(token, path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) });

export type StreamEvent =
  | { event: 'meta'; data: { conversation_id: string; message_id: string } }
  | { event: 'status'; data: { stage: 'retrieving' | 'generating' | 'preparing_action' } }
  | { event: 'delta'; data: { text: string } }
  | { event: 'sources'; data: { sources: Source[] } }
  | { event: 'action'; data: { action: PendingAction } }
  | { event: 'done'; data: { finish_reason: string } }
  | { event: 'error'; data: { code: string; message: string } };

/** POST an SSE endpoint and call onEvent for each event until the stream ends or signal aborts. */
export async function stream(token: string, path: string, body: unknown, onEvent: (e: StreamEvent) => void, signal?: AbortSignal) {
  const response = await fetch(path, {
    method: 'POST', signal,
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', Accept: 'text/event-stream' },
    body: JSON.stringify(body ?? {}),
  });
  if (response.status === 401) dispatchEvent(new Event('ai-buddy:unauthorized'));
  if (!response.ok || !response.body) {
    let message = `Ошибка ${response.status}`;
    try { message = (await response.json()).error || message; } catch { /* not JSON */ }
    throw new BotError(response.status, message);
  }
  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += value;
    let cut;
    while ((cut = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, cut); buffer = buffer.slice(cut + 2);
      let event = 'message'; let data = '';
      for (const line of block.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim();
        else if (line.startsWith('data:')) data += line.slice(5).trim();
      }
      if (data) { try { onEvent({ event, data: JSON.parse(data) } as StreamEvent); } catch { /* skip malformed */ } }
    }
  }
}

export const bot = {
  chat: (token: string, body: { message: string; conversation_id?: string; page_context?: PageContext }, onEvent: (e: StreamEvent) => void, signal?: AbortSignal) =>
    stream(token, '/api/chat/stream', body, onEvent, signal),
  brief: (token: string, onEvent: (e: StreamEvent) => void, signal?: AbortSignal) => stream(token, '/api/chat/brief/stream', {}, onEvent, signal),
  messages: (token: string, conversationId?: string) =>
    request<{ conversation_id: string | null; messages: ChatMessage[] }>(token, `/api/chat/messages${conversationId ? `?conversation_id=${conversationId}` : ''}`),
  feedback: (token: string, messageId: string, rating: 'up' | 'down', comment?: string) => post<void>(token, `/api/chat/messages/${messageId}/feedback`, comment ? { rating, comment } : { rating }),
  confirm: (token: string, actionId: string) => post<ActionResult>(token, `/api/actions/${actionId}/confirm`),
  cancel: (token: string, actionId: string) => post<PendingAction>(token, `/api/actions/${actionId}/cancel`),
  requests: (token: string) => request<ServiceRequest[]>(token, '/api/requests'),
  hints: (token: string) => request<Hint[]>(token, '/api/hints'),
  readHint: (token: string, id: string) => post<void>(token, `/api/hints/${id}/read`),
  // buddy / teamlead
  questions: (token: string, status?: 'open' | 'answered') => request<MentorQuestion[]>(token, `/api/buddy/questions${status ? `?status=${status}` : ''}`),
  draft: (token: string, id: string) => request<MentorDraft>(token, `/api/buddy/questions/${id}/draft`),
  answer: (token: string, id: string, answer: string) => post<MentorQuestion>(token, `/api/buddy/questions/${id}/answer`, { answer }),
  staffRequests: (token: string) => request<{ request: ServiceRequest; intern_id: string; intern_name: string }[]>(token, '/api/buddy/requests'),
  decide: (token: string, id: string, status: 'approved' | 'rejected') => post<ServiceRequest>(token, `/api/buddy/requests/${id}/decision`, { status }),
  metrics: (token: string) => request<ChatMetrics>(token, '/api/metrics/chat'),
  gaps: (token: string) => request<GapsReport>(token, '/api/analytics/gaps'),
  lint: (token: string) => request<KbLintReport>(token, '/api/kb/lint'),
};

export const REQUEST_STATUS: Record<ServiceRequest['status'], string> = {
  pending_approval: 'на согласовании', approved: 'согласована', rejected: 'отклонена',
};

export const ownQuestions = (token: string) => request<MentorQuestion[]>(token, '/api/questions');
