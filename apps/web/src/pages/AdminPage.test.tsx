import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { StaffShell } from '../components/staff/StaffShell';
import type { AdminAccount, AdminLlm, AdminNotification, AdminSystem, StuckCheck } from '../bot/adminApi';
import { buddyUser, internUser } from '../test/fixtures';
import { AdminPage } from './AdminPage';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return { ...actual, getAssignedInterns: vi.fn(), getPendingSubmissions: vi.fn(), getInternProgress: vi.fn() };
});

const account = (over: Partial<AdminAccount>): AdminAccount => ({
  id: 'a1', email: 'x@demo.example', first_name: '', last_name: '', role: 'intern', profession: 'backend', team: '', department: '',
  buddy_name: '', teamlead_name: '', created_at: '2026-11-01T10:00:00Z', last_login_at: null, active_sessions: 0, ...over,
});
const ACCOUNTS = [
  account({ id: 'a1', email: 'intern@demo.example', first_name: 'Аня', last_name: 'Стажёрова', buddy_name: 'Олег Наставников', teamlead_name: 'Ирина Руководова', active_sessions: 2 }),
  account({ id: 'a2', email: 'buddy@demo.example', role: 'buddy', profession: '' }),
  account({ id: 'a3', email: 'teamlead@demo.example', role: 'teamlead', profession: '' }),
];
const LLM: AdminLlm = {
  provider: 'openrouter', mode: 'llm', model: 'qwen/qwen-flash', fallback_model: '',
  balance: { available: true, usage: 2.5, limit: 10, limit_remaining: 7.5, usage_daily: 0.1, usage_weekly: 0.5, usage_monthly: 2.5, is_free_tier: false, label: null, note: '' },
  budget: { daily_tokens_limit: 1000, daily_tokens_used: 250, per_minute_limit: 20 },
};
const SYSTEM: AdminSystem = {
  checks: [
    { id: 'database', title: 'База данных', status: 'ok', detail: 'PostgreSQL отвечает', latency_ms: 2 },
    { id: 'ai', title: 'AI-сервис', status: 'error', detail: 'AI-сервис недоступен', latency_ms: 30 },
  ],
  counters: { users_by_role: { intern: 5, buddy: 2, teamlead: 1 }, open_questions: 3, pending_requests: 4, messages_today: 7, active_sessions: 6 },
  hints: { pending: 1, processing: 0, ready: 2, unread: 1 },
};
const sample = (text: string, ask?: string) => ({ text, ask });
const NOTIFICATIONS: AdminNotification[] = [
  { id: 'greeting', title: 'Приветствие дня', source: 'widget', when: 'Первое открытие страницы за день', cadence: 'Раз в день', button: 'Показать', check: 'preview', samples: [sample('Привет! У меня есть сводка на сегодня.', 'Покажи сводку')], live: [] },
  { id: 'stuck_task', title: 'Задача идёт дольше плана', source: 'worker', when: 'Задача активна дольше плана', cadence: 'Раз в минуту', check: 'backend', samples: [sample('Задача идёт дольше плана')], live: [{ label: 'Просроченных активных задач сейчас', value: 3 }] },
];
const STUCK: StuckCheck = {
  overdue_tasks: 3, pending_before: 2, ready_before: 0, pending_after: 0, ready_after: 2,
  tasks: [{ task: 'Долгая задача', user_email: 'intern@demo.example', hint_status: 'ready', hint_text: 'Начните с малого' }],
};

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
let calls: string[] = [];
let routes: Record<string, () => Response>;

beforeEach(() => {
  calls = [];
  routes = {
    'GET /api/admin/accounts': () => json({ accounts: ACCOUNTS }),
    'GET /api/admin/llm': () => json(LLM),
    'POST /api/admin/llm/ping': () => json({ ok: true, latency_ms: 321, answer: 'pong', input_tokens: 12, output_tokens: 2 }),
    'GET /api/admin/system': () => json(SYSTEM),
    'POST /api/admin/kb/reload': () => json({ documents: 4, chunks: 12, source: 'markdown' }),
    'GET /api/admin/notifications': () => json({ items: NOTIFICATIONS }),
    'POST /api/admin/notifications/stuck_task/check': () => json(STUCK),
  };
  vi.stubGlobal('fetch', vi.fn(async (path: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${path}`;
    calls.push(key);
    return routes[key]?.() ?? json([]);
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe('админ-панель', () => {
  it('показывает аккаунты и фильтрует по роли', async () => {
    render(<AdminPage token="t" />);
    expect(await screen.findByText('Стажёрова Аня')).toBeInTheDocument();
    expect(screen.getByText('Олег Наставников')).toBeInTheDocument();
    expect(screen.getByText('teamlead@demo.example')).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('Роль'), 'buddy');
    expect(screen.getByText('buddy@demo.example')).toBeInTheDocument();
    expect(screen.queryByText('Стажёрова Аня')).not.toBeInTheDocument();
    expect(screen.queryByText('teamlead@demo.example')).not.toBeInTheDocument();
  });

  it('на вкладке «API и модель» показывает баланс и проверяет модель', async () => {
    render(<AdminPage token="t" />);
    await userEvent.click(screen.getByRole('tab', { name: 'API и модель' }));
    expect(await screen.findByText('qwen/qwen-flash')).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Использование лимита' })).toHaveAttribute('aria-valuenow', '25');
    expect(screen.getByRole('progressbar', { name: 'Дневной бюджет токенов' })).toHaveAttribute('aria-valuenow', '25');
    await userEvent.click(screen.getByRole('button', { name: 'Проверить модель' }));
    const result = await screen.findByText(/Модель отвечает/);
    expect(result.parentElement).toHaveTextContent('«pong»');
    expect(result.parentElement).toHaveTextContent('321 мс');
    expect(result.parentElement).toHaveTextContent('12 на вход, 2 на выход');
  });

  it('объясняет, если баланс недоступен', async () => {
    routes['GET /api/admin/llm'] = () => json({ ...LLM, provider: 'mock', mode: 'mock', model: '', budget: null,
      balance: { ...LLM.balance, available: false, note: 'Режим mock: внешний LLM не используется, баланса нет.' } });
    render(<AdminPage token="t" />);
    await userEvent.click(screen.getByRole('tab', { name: 'API и модель' }));
    expect(await screen.findByText(/баланса нет/)).toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('оповещения: пример уходит в Бади с уникальным id, проверка воркера показывает результат', async () => {
    const previews: { id: string; text: string; ask?: string }[] = [];
    const listener = (e: Event) => previews.push((e as CustomEvent).detail);
    window.addEventListener('ai-buddy:nudge-preview', listener);
    try {
      render(<AdminPage token="t" />);
      await userEvent.click(screen.getByRole('tab', { name: 'Оповещения' }));
      const greeting = await screen.findByRole('article', { name: 'Приветствие дня' });
      await userEvent.click(within(greeting).getByRole('button', { name: 'Проверить' }));
      await userEvent.click(within(greeting).getByRole('button', { name: 'Проверить' }));
      expect(previews).toHaveLength(2);
      expect(previews[0]).toMatchObject({ text: 'Привет! У меня есть сводка на сегодня.', ask: 'Покажи сводку' });
      expect(previews[0].id).toMatch(/^preview-\d+$/);
      expect(previews[1].id).not.toBe(previews[0].id);
      expect(calls).not.toContain('POST /api/admin/notifications/stuck_task/check');

      const stuck = screen.getByRole('article', { name: 'Задача идёт дольше плана' });
      expect(within(stuck).getByText(/Просроченных активных задач сейчас/)).toHaveTextContent('3');
      await userEvent.click(within(stuck).getByRole('button', { name: 'Проверить' }));
      expect(await within(stuck).findByText(/Тик воркера выполнен/)).toBeInTheDocument();
      expect(within(stuck).getByText('Долгая задача')).toBeInTheDocument();
      expect(within(stuck).getByText('Начните с малого')).toBeInTheDocument();
      expect(previews).toHaveLength(2);
    } finally {
      window.removeEventListener('ai-buddy:nudge-preview', listener);
    }
  });

  it('система: статусы проверок, счётчики и перечитывание базы знаний', async () => {
    render(<AdminPage token="t" />);
    await userEvent.click(screen.getByRole('tab', { name: 'Система' }));
    expect(await screen.findByText('PostgreSQL отвечает')).toBeInTheDocument();
    expect(screen.getByText('работает')).toBeInTheDocument();
    expect(screen.getByText('сбой')).toBeInTheDocument();
    expect(screen.getByText('Заявок на согласовании').previousSibling).toHaveTextContent('4');
    await userEvent.click(screen.getByRole('button', { name: 'Перечитать базу знаний' }));
    expect(await screen.findByText(/4 документов, 12 фрагментов/)).toBeInTheDocument();
    expect(calls).toContain('POST /api/admin/kb/reload');
  });

  it('показывает ошибку, если бэкенд ответил отказом', async () => {
    routes['GET /api/admin/accounts'] = () => json({ error: 'forbidden' }, 403);
    render(<AdminPage token="t" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('forbidden');
  });
});

describe('пункт «Админ» в меню', () => {
  beforeEach(() => {
    vi.mocked(client.getAssignedInterns).mockResolvedValue([internUser]);
    vi.mocked(client.getPendingSubmissions).mockResolvedValue([]);
    vi.mocked(client.getInternProgress).mockRejectedValue(new Error('нет данных'));
  });

  it('есть у руководителя и открывает панель', async () => {
    render(<StaffShell user={{ ...buddyUser, role: 'teamlead' }} token="t" onLogout={() => undefined} />);
    await userEvent.click(screen.getByRole('button', { name: 'Админ' }));
    expect(await screen.findByRole('heading', { name: 'Админ' })).toBeInTheDocument();
    await waitFor(() => expect(calls).toContain('GET /api/admin/accounts'));
  });

  it('нет у наставника', () => {
    render(<StaffShell user={buddyUser} token="t" onLogout={() => undefined} />);
    expect(screen.queryByRole('button', { name: 'Админ' })).not.toBeInTheDocument();
  });
});
