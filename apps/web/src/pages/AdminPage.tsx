import { useCallback, useEffect, useState } from 'react';
import {
  admin, type AdminAccount, type AnswerFeedback, type AdminLlm, type AdminNotification, type AdminPing, type AdminSystem,
  type KbCounts, type NotificationSample, type StuckCheck,
} from '../bot/adminApi';
import type { GapsReport, KbLintReport } from '../bot/api';
import { personName, staffRoleLabel } from '../lib/format';

/** Админ-панель руководителя: аккаунты, API и модель, оповещения Бади, состояние системы. Только чтение и проверки. */

const TABS = [
  { id: 'accounts', label: 'Аккаунты' },
  { id: 'llm', label: 'API и модель' },
  { id: 'notifications', label: 'Оповещения' },
  { id: 'quality', label: 'Оценки ответов' },
  { id: 'kb', label: 'База знаний' },
  { id: 'system', label: 'Система' },
] as const;
type TabId = (typeof TABS)[number]['id'];

const errorText = (e: unknown) => (e instanceof Error ? e.message : 'Ошибка');
const date = (iso: string | null) => (iso ? new Date(iso).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');
const usd = (n: number | null | undefined) => (n === null || n === undefined ? '—' : `$${n.toFixed(n < 1 ? 4 : 2)}`);
const PROFESSIONS: Record<string, string> = { backend: 'Backend', frontend: 'Frontend', ml: 'ML' };

/** Загрузка данных вкладки: данные, ошибка, «идёт загрузка» и перезапуск. */
function useLoad<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const run = useCallback(async () => {
    setLoading(true);
    try { setData(await load()); setError(''); } catch (e) { setError(errorText(e)); } finally { setLoading(false); }
  }, [load]);
  useEffect(() => { void run(); }, [run]);
  return { data, error, loading, reload: run };
}

function Status({ loading, error }: { loading: boolean; error: string }) {
  if (error) return <p className="notice error" role="alert">{error}</p>;
  return loading ? <p className="muted" role="status">Загружаем…</p> : null;
}

// ───────── Аккаунты ─────────

const ROLE_FILTERS = [
  { id: 'all', label: 'Все роли' }, { id: 'intern', label: 'Стажёры' },
  { id: 'buddy', label: 'Наставники' }, { id: 'teamlead', label: 'Руководители' },
] as const;

function AccountsTab({ token }: { token: string }) {
  const { data, error, loading, reload } = useLoad<AdminAccount[]>(useCallback(() => admin.accounts(token), [token]));
  const [role, setRole] = useState<string>('all');
  const [notice, setNotice] = useState('');
  const [busyId, setBusyId] = useState('');
  const rows = (data ?? []).filter(a => role === 'all' || a.role === role);
  // стажёр увидит приветственный тур при следующем входе: бэкенд сбрасывает «первый вход»
  const restartOnboarding = async (a: AdminAccount) => {
    setBusyId(a.id); setNotice('');
    try {
      await admin.restartOnboarding(token, a.id);
      setNotice(`Онбординг запущен: ${personName(a)} увидит приветственный тур при следующем входе.`);
      await reload();
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : 'Не удалось запустить онбординг'); }
    finally { setBusyId(''); }
  };
  return (
    <section className="staff-card admin-block" aria-label="Аккаунты">
      <div className="panel-head">
        <h2>Аккаунты <span className="count-badge">{rows.length}</span></h2>
        <div className="admin-actions">
          <label className="field admin-filter">
            <span className="admin-sr">Роль</span>
            <select aria-label="Роль" value={role} onChange={e => setRole(e.target.value)}>
              {ROLE_FILTERS.map(f => <option key={f.id} value={f.id}>{f.label}</option>)}
            </select>
          </label>
          <button className="pill pill-black pill-sm" onClick={() => void reload()} disabled={loading}>Обновить</button>
        </div>
      </div>
      <Status loading={loading && !data} error={error} />
      {notice && <p className="notice" role="status">{notice}</p>}
      <ul className="admin-list">
        {rows.map(a => {
          const name = personName(a);
          return (
            <li key={a.id} className="admin-row">
              <div className="admin-cell admin-main">
                {a.first_name || a.last_name ? <><b>{name}</b><small className="muted">{a.email}</small></> : <b>{a.email}</b>}
              </div>
              <div className="admin-cell" data-label="Роль"><span className="chip">{staffRoleLabel(a.role)}</span></div>
              <div className="admin-cell" data-label="Профессия">{PROFESSIONS[a.profession] ?? '—'}</div>
              <div className="admin-cell" data-label="Команда">{[a.department, a.team].filter(Boolean).join(' / ') || '—'}</div>
              <div className="admin-cell" data-label="Наставник">{a.buddy_name || '—'}</div>
              <div className="admin-cell" data-label="Руководитель">{a.teamlead_name || '—'}</div>
              <div className="admin-cell" data-label="Создан">{date(a.created_at)}</div>
              <div className="admin-cell" data-label="Последний вход">{date(a.last_login_at)}</div>
              <div className="admin-cell" data-label="Сессии"><span className={`chip${a.active_sessions ? ' completed' : ''}`}>{a.active_sessions}</span></div>
              <div className="admin-cell admin-action">
                {a.role === 'intern' && (
                  <button className="pill pill-light pill-sm" disabled={busyId === a.id} onClick={() => void restartOnboarding(a)}>
                    {busyId === a.id ? 'Запускаем…' : 'Запустить онбординг'}
                  </button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      {!loading && !error && rows.length === 0 && <p className="muted">Аккаунтов с такой ролью нет.</p>}
    </section>
  );
}

// ───────── API и модель ─────────

function LlmTab({ token }: { token: string }) {
  const { data, error, loading, reload } = useLoad<AdminLlm>(useCallback(() => admin.llm(token), [token]));
  const [ping, setPing] = useState<AdminPing | null>(null);
  const [pinging, setPinging] = useState(false);
  const [pingError, setPingError] = useState('');
  const runPing = async () => {
    setPinging(true); setPingError(''); setPing(null);
    try { setPing(await admin.ping(token)); } catch (e) { setPingError(errorText(e)); } finally { setPinging(false); }
  };
  const b = data?.balance;
  const percent = b && b.limit ? Math.min(100, Math.round(((b.usage ?? 0) / b.limit) * 100)) : null;
  const budget = data?.budget;
  const budgetPercent = budget && budget.daily_tokens_limit ? Math.min(100, Math.round((budget.daily_tokens_used / budget.daily_tokens_limit) * 100)) : null;
  return (
    <div className="admin-stack">
      <section className="staff-card admin-block" aria-label="Модель">
        <div className="panel-head">
          <h2>Провайдер и модель</h2>
          <button className="pill pill-black pill-sm" onClick={() => void reload()} disabled={loading}>Обновить</button>
        </div>
        <Status loading={loading && !data} error={error} />
        {data && (
          <dl className="admin-facts">
            <div><dt>Провайдер</dt><dd>{data.provider}</dd></div>
            <div><dt>Режим</dt><dd><span className={`chip${data.mode === 'llm' ? ' completed' : ''}`}>{data.mode === 'llm' ? 'LLM' : 'mock'}</span></dd></div>
            <div><dt>Модель</dt><dd>{data.model || '—'}</dd></div>
            <div><dt>Запасная модель</dt><dd>{data.fallback_model || '—'}</dd></div>
          </dl>
        )}
      </section>

      {data && b && (
        <section className="staff-card admin-block" aria-label="Баланс API">
          <h2>Баланс API</h2>
          {b.available ? (
            <>
              <dl className="admin-facts">
                <div><dt>Потрачено всего</dt><dd>{usd(b.usage)}</dd></div>
                <div><dt>Лимит</dt><dd>{b.limit === null ? 'без лимита' : usd(b.limit)}</dd></div>
                <div><dt>Осталось</dt><dd>{b.limit === null ? '—' : usd(b.limit_remaining)}</dd></div>
                <div><dt>За сутки</dt><dd>{usd(b.usage_daily)}</dd></div>
                <div><dt>За неделю</dt><dd>{usd(b.usage_weekly)}</dd></div>
                <div><dt>За месяц</dt><dd>{usd(b.usage_monthly)}</dd></div>
              </dl>
              {percent !== null && (
                <div className="admin-bar" role="progressbar" aria-label="Использование лимита" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
                  <span style={{ width: `${percent}%` }} /><em>{percent}% лимита</em>
                </div>
              )}
              {b.is_free_tier && <p className="muted">Бесплатный тариф: возможны ограничения по запросам.</p>}
            </>
          ) : null}
          {b.note && <p className="notice">{b.note}</p>}
        </section>
      )}

      {budget && (
        <section className="staff-card admin-block" aria-label="Токены за сутки">
          <h2>Токены за сутки (UTC)</h2>
          {budget.daily_tokens_limit ? (
            <>
              <p>Потрачено {budget.daily_tokens_used.toLocaleString('ru-RU')} из {budget.daily_tokens_limit.toLocaleString('ru-RU')}</p>
              <div className="admin-bar" role="progressbar" aria-label="Дневной бюджет токенов" aria-valuemin={0} aria-valuemax={100} aria-valuenow={budgetPercent ?? 0}>
                <span style={{ width: `${budgetPercent}%` }} /><em>{budgetPercent}%</em>
              </div>
            </>
          ) : <p className="muted">Дневной лимит токенов не задан. Потрачено сегодня: {budget.daily_tokens_used.toLocaleString('ru-RU')}.</p>}
          <p className="muted">Лимит запросов на пользователя: {budget.per_minute_limit ? `${budget.per_minute_limit} в минуту` : 'нет'}.</p>
        </section>
      )}

      <section className="staff-card admin-block" aria-label="Проверка модели">
        <div className="panel-head">
          <h2>Проверка модели</h2>
          <button className="pill pill-black pill-sm" onClick={() => void runPing()} disabled={pinging}>{pinging ? 'Проверяем…' : 'Проверить модель'}</button>
        </div>
        <p className="muted">Короткий запрос «Ответь одним словом: pong» с теми же настройками, что у чата.</p>
        {pingError && <p className="notice error" role="alert">{pingError}</p>}
        {ping && (
          <div className={`notice ${ping.ok ? 'success' : 'error'}`} role="status">
            {ping.ok ? (
              <>
                <b>Модель отвечает.</b> Ответ: «{ping.answer}». Задержка: {ping.latency_ms} мс.
                {' '}Токены: {ping.input_tokens ?? 0} на вход, {ping.output_tokens ?? 0} на выход.
              </>
            ) : <><b>Модель не ответила.</b> {ping.error || 'Неизвестная ошибка.'} Задержка: {ping.latency_ms} мс.</>}
          </div>
        )}
      </section>
    </div>
  );
}

// ───────── Оповещения ─────────

let previewCounter = 0;
/** Показывает пример всплывашки в собственном Бади администратора (его слушает BuddyHost). */
export function previewNudge(sample: NotificationSample) {
  previewCounter += 1;
  window.dispatchEvent(new CustomEvent('ai-buddy:nudge-preview', {
    detail: { id: `preview-${previewCounter}`, text: sample.text, detail: sample.detail, ask: sample.ask },
  }));
}

const SOURCE_LABEL: Record<AdminNotification['source'], string> = { widget: 'виджет', site: 'сайт', worker: 'воркер бэкенда + сайт' };
const HINT_STATUS: Record<string, string> = { none: 'нет подсказки', pending: 'в очереди', processing: 'готовится', ready: 'готова' };

function StuckResult({ result }: { result: StuckCheck }) {
  return (
    <div className="admin-result" role="status">
      <p>
        <b>Тик воркера выполнен.</b> Просроченных активных задач: {result.overdue_tasks}.
        {' '}Подсказки в очереди: {result.pending_before} → {result.pending_after}, готовых: {result.ready_before} → {result.ready_after}.
      </p>
      {result.error && <p className="notice error">{result.error}</p>}
      {result.tasks.length > 0 && (
        <ul className="admin-mini">
          {result.tasks.map((t, i) => (
            <li key={i}>
              <b>{t.task}</b> <small className="muted">{t.user_email} · {HINT_STATUS[t.hint_status] ?? t.hint_status}</small>
              {t.hint_text && <p className="muted">{t.hint_text}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function NotificationCard({ item, token, onChecked }: { item: AdminNotification; token: string; onChecked: () => void }) {
  const [result, setResult] = useState<StuckCheck | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [shown, setShown] = useState('');
  const backend = item.check === 'backend';
  const check = async (sample?: NotificationSample) => {
    setError(''); setShown('');
    if (!backend) {
      if (sample) { previewNudge(sample); setShown('Пример показан в вашем Бади.'); }
      return;
    }
    setBusy(true);
    try { setResult(await admin.checkStuckTask(token)); onChecked(); } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  const single = item.samples.length === 1;
  return (
    <article className="staff-card admin-block admin-notice" aria-label={item.title}>
      <div className="panel-head">
        <h2>{item.title}</h2>
        <span className="chip">{SOURCE_LABEL[item.source]}</span>
      </div>
      <dl className="admin-facts admin-facts-wide">
        <div><dt>Когда срабатывает</dt><dd>{item.when}</dd></div>
        <div><dt>Как часто</dt><dd>{item.cadence}</dd></div>
        {item.button && <div><dt>Кнопка</dt><dd>«{item.button}»</dd></div>}
      </dl>
      {item.live.length > 0 && (
        <div className="admin-live">
          {item.live.map(l => <span key={l.label} className="chip">{l.label}: <b>{l.value}</b></span>)}
        </div>
      )}
      <ul className="admin-samples">
        {item.samples.map((s, i) => (
          <li key={i}>
            <div>
              {s.trigger && <small className="muted">Шаблоны: {s.trigger}</small>}
              <p>«{s.text}»</p>
              {s.detail && <p className="muted">{s.detail}</p>}
            </div>
            {!single && !backend && <button className="pill pill-black pill-sm" onClick={() => void check(s)}>Проверить</button>}
          </li>
        ))}
      </ul>
      {(single || backend) && (
        <button className="pill pill-black pill-sm" disabled={busy} onClick={() => void check(item.samples[0])}>
          {busy ? 'Проверяем…' : 'Проверить'}
        </button>
      )}
      {shown && <p className="muted" role="status">{shown}</p>}
      {error && <p className="notice error" role="alert">{error}</p>}
      {result && <StuckResult result={result} />}
    </article>
  );
}

function NotificationsTab({ token }: { token: string }) {
  const { data, error, loading, reload } = useLoad<AdminNotification[]>(useCallback(() => admin.notifications(token), [token]));
  return (
    <div className="admin-stack">
      <div className="panel-head admin-plain-head">
        <p className="muted">Всплывашки Бади: откуда берутся, когда срабатывают и как их проверить.</p>
        <button className="pill pill-black pill-sm" onClick={() => void reload()} disabled={loading}>Обновить числа</button>
      </div>
      <Status loading={loading && !data} error={error} />
      {(data ?? []).map(item => <NotificationCard key={item.id} item={item} token={token} onChecked={() => void reload()} />)}
    </div>
  );
}

// ───────── Система ─────────

const COUNTER_LABELS: [keyof AdminSystem['counters'], string][] = [
  ['open_questions', 'Открытых вопросов'], ['pending_requests', 'Заявок на согласовании'],
  ['messages_today', 'Сообщений в чате сегодня'], ['active_sessions', 'Активных сессий'],
];

function QualityTab({ token }: { token: string }) {
  const { data, error, loading, reload } = useLoad<AnswerFeedback>(useCallback(() => admin.feedback(token), [token]));
  return (
    <div className="admin-stack">
      <section className="staff-card admin-block" aria-label="Оценки ответов бота">
        <div className="panel-head">
          <h2>Оценки ответов бота</h2>
          <div className="admin-actions"><button className="pill pill-black pill-sm" onClick={() => void reload()} disabled={loading}>Обновить</button></div>
        </div>
        <p className="muted">Оценки 👍/👎 стажёров под ответами. Это сигнал о качестве бота: наставники их не видят.</p>
        <Status loading={loading && !data} error={error} />
        {data && (
          <>
            <div className="admin-counters">
              <div><b>{data.up}</b><span>👍 помогло</span></div>
              <div><b>{data.down}</b><span>👎 не помогло</span></div>
            </div>
            {data.items.length === 0
              ? <p className="muted">Комментариев к 👎 пока нет.</p>
              : <ul className="admin-checks">{data.items.map(i => <li key={i.message_id}><div><b>{i.comment}</b><small className="muted">{date(i.created_at)}</small></div></li>)}</ul>}
          </>
        )}
      </section>
    </div>
  );
}

function KbTab({ token }: { token: string }) {
  const [gaps, setGaps] = useState<GapsReport | null>(null);
  const [lint, setLint] = useState<KbLintReport | null>(null);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const run = async (kind: 'gaps' | 'lint') => {
    setBusy(kind); setError('');
    try { if (kind === 'gaps') setGaps(await admin.kbGaps(token)); else setLint(await admin.kbLint(token)); }
    catch (e) { setError(errorText(e)); } finally { setBusy(''); }
  };
  return (
    <div className="admin-stack">
      <section className="staff-card admin-block" aria-label="Качество базы знаний">
        <h2>Качество базы знаний</h2>
        <p className="muted">Где бот не справляется: темы вопросов без ответа и противоречия между документами.</p>
        <div className="admin-actions">
          <button className="pill pill-black pill-sm" disabled={busy === 'gaps'} onClick={() => void run('gaps')}>{busy === 'gaps' ? 'Считаю…' : 'Пробелы: темы вопросов без ответа'}</button>
          <button className="pill pill-black pill-sm" disabled={busy === 'lint'} onClick={() => void run('lint')}>{busy === 'lint' ? 'Проверяю (до минуты)…' : 'Линтер: противоречия и устаревшее'}</button>
        </div>
        {error && <p className="notice error" role="alert">{error}</p>}
        {gaps && (gaps.clusters.length
          ? <ul>{gaps.clusters.map(c => <li key={c.label}><b>{c.label}</b> — {c.count}: <span className="muted">{c.examples.join(' / ')}</span></li>)}</ul>
          : <p className="muted">Эскалированных вопросов пока нет.</p>)}
        {lint && (lint.issues.length
          ? <ul>{lint.issues.map((i, n) => <li key={n}><span className="chip">{i.kind === 'contradiction' ? 'противоречие' : i.kind === 'stale' ? 'устарело' : i.kind}</span> <b>{i.doc_ids.join(' ↔ ')}</b>: {i.detail}</li>)}</ul>
          : <p className="muted">Проблем не найдено.</p>)}
      </section>
    </div>
  );
}

function SystemTab({ token }: { token: string }) {
  const { data, error, loading, reload } = useLoad<AdminSystem>(useCallback(() => admin.system(token), [token]));
  const [reloading, setReloading] = useState(false);
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null);
  const reloadKb = async () => {
    setReloading(true); setNote(null);
    try {
      const r: KbCounts = await admin.reloadKb(token);
      setNote({ ok: true, text: `База знаний перечитана: ${r.documents} документов, ${r.chunks} фрагментов.` });
      void reload();
    } catch (e) { setNote({ ok: false, text: errorText(e) }); } finally { setReloading(false); }
  };
  return (
    <div className="admin-stack">
      <section className="staff-card admin-block" aria-label="Состояние системы">
        <div className="panel-head">
          <h2>Состояние системы</h2>
          <div className="admin-actions">
            <button className="pill pill-black pill-sm" onClick={() => void reloadKb()} disabled={reloading}>{reloading ? 'Перечитываем…' : 'Перечитать базу знаний'}</button>
            <button className="pill pill-black pill-sm" onClick={() => void reload()} disabled={loading}>Обновить</button>
          </div>
        </div>
        {note && <p className={`notice ${note.ok ? 'success' : 'error'}`} role="status">{note.text}</p>}
        <Status loading={loading && !data} error={error} />
        {data && (
          <ul className="admin-checks">
            {data.checks.map(c => (
              <li key={c.id}>
                <span className={`chip${c.status === 'ok' ? ' completed' : ' pending'}`}>{c.status === 'ok' ? 'работает' : 'сбой'}</span>
                <div><b>{c.title}</b><small className="muted">{c.detail}</small></div>
                <small className="muted">{c.latency_ms} мс</small>
              </li>
            ))}
          </ul>
        )}
      </section>
      {data && (
        <section className="staff-card admin-block" aria-label="Счётчики">
          <h2>Счётчики</h2>
          <div className="admin-counters">
            {[['Стажёров', data.counters.users_by_role.intern ?? 0], ['Наставников', data.counters.users_by_role.buddy ?? 0],
              ['Руководителей', data.counters.users_by_role.teamlead ?? 0],
              ...COUNTER_LABELS.map(([key, label]) => [label, data.counters[key] as number] as [string, number])]
              .map(([label, value]) => <div key={label as string}><b>{value as number}</b><span>{label as string}</span></div>)}
          </div>
        </section>
      )}
    </div>
  );
}

export function AdminPage({ token }: { token: string }) {
  const [tab, setTab] = useState<TabId>('accounts');
  return (
    <div className="home staff-page admin-page">
      <header className="greeting">
        <h1>Админ</h1>
        <p>Аккаунты, модель и её баланс, оповещения Бади и состояние системы</p>
      </header>
      <div className="desk-tabs admin-tabs" role="tablist" aria-label="Разделы админ-панели">
        {TABS.map(t => (
          <button key={t.id} role="tab" aria-selected={tab === t.id} className={`pill pill-sm ${tab === t.id ? 'pill-black' : 'pill-light'}`} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      <div role="tabpanel">
        {tab === 'accounts' && <AccountsTab token={token} />}
        {tab === 'llm' && <LlmTab token={token} />}
        {tab === 'notifications' && <NotificationsTab token={token} />}
        {tab === 'quality' && <QualityTab token={token} />}
        {tab === 'kb' && <KbTab token={token} />}
        {tab === 'system' && <SystemTab token={token} />}
      </div>
    </div>
  );
}
