// Admin panel API (teamlead only). Contract: contracts/backend.openapi.json, paths /api/admin/*.
import { post, request, type GapsReport, type KbLintReport } from './api';

export type AdminAccount = {
  id: string; email: string; first_name: string; last_name: string; role: 'intern' | 'buddy' | 'teamlead';
  profession: string; team: string; department: string; buddy_name: string; teamlead_name: string;
  created_at: string; last_login_at: string | null; active_sessions: number;
};
export type AdminBalance = {
  available: boolean; currency?: string; usage: number | null; limit: number | null; limit_remaining: number | null;
  usage_daily: number | null; usage_weekly: number | null; usage_monthly: number | null;
  is_free_tier: boolean | null; label: string | null; note: string;
};
export type AdminLlm = {
  provider: string; mode: 'mock' | 'llm'; model: string; fallback_model?: string; balance: AdminBalance;
  budget: { daily_tokens_limit: number; daily_tokens_used: number; per_minute_limit: number } | null;
};
export type AdminPing = { ok: boolean; mode?: string; model?: string; latency_ms: number; answer: string; input_tokens?: number; output_tokens?: number; error?: string };
export type AdminCheck = { id: string; title: string; status: 'ok' | 'error'; detail: string; latency_ms: number };
export type AdminSystem = {
  checks: AdminCheck[];
  counters: { users_by_role: Record<string, number>; open_questions: number; pending_requests: number; messages_today: number; active_sessions: number };
  hints: Record<string, number>;
};
export type NotificationSample = { trigger?: string; text: string; detail?: string; ask?: string };
export type AdminNotification = {
  id: string; title: string; source: 'widget' | 'site' | 'worker'; when: string; cadence: string; button?: string;
  check: 'preview' | 'backend'; samples: NotificationSample[]; live: { label: string; value: number }[];
};
export type StuckCheck = {
  overdue_tasks: number; pending_before: number; ready_before: number; pending_after: number; ready_after: number; error?: string;
  tasks: { task: string; user_email: string; hint_status: string; hint_text: string }[];
};
export type KbCounts = { documents: number; chunks: number; source?: string };

export type AnswerFeedback = { up: number; down: number; items: { message_id: string; comment: string; created_at: string }[] };

export const admin = {
  access: (token: string) => request<{ allowed: boolean }>(token, '/api/admin/access').then(r => r.allowed).catch(() => false),
  feedback: (token: string) => request<AnswerFeedback>(token, '/api/admin/feedback'),
  kbGaps: (token: string) => request<GapsReport>(token, '/api/admin/kb/gaps'),
  kbLint: (token: string) => request<KbLintReport>(token, '/api/admin/kb/lint'),
  accounts: (token: string) => request<{ accounts: AdminAccount[] }>(token, '/api/admin/accounts').then(r => r.accounts),
  restartOnboarding: (token: string, userId: string) => request<void>(token, `/api/admin/accounts/${userId}/onboarding`, { method: 'POST' }),
  llm: (token: string) => request<AdminLlm>(token, '/api/admin/llm'),
  ping: (token: string) => post<AdminPing>(token, '/api/admin/llm/ping'),
  system: (token: string) => request<AdminSystem>(token, '/api/admin/system'),
  reloadKb: (token: string) => post<KbCounts>(token, '/api/admin/kb/reload'),
  notifications: (token: string) => request<{ items: AdminNotification[] }>(token, '/api/admin/notifications').then(r => r.items),
  checkStuckTask: (token: string) => post<StuckCheck>(token, '/api/admin/notifications/stuck_task/check'),
};
