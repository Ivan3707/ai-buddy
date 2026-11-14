import { useCallback, useEffect, useState } from 'react';
import { ApiError, apiRequest, type User } from '@ai-buddy/client';
import { formatDateTime, personName } from '../lib/format';

export type PortalRoute = { kind: 'assistant' | 'corporate'; id: string } | { kind: 'list' };

/** /portal/requests -> список, /portal/requests/<kind>/<id> -> заявка, иначе null. */
export function parsePortalPath(pathname: string): PortalRoute | null {
  if (/^\/portal\/requests\/?$/.test(pathname)) return { kind: 'list' };
  const match = /^\/portal\/requests\/(assistant|corporate)\/([\w-]{8,64})\/?$/.exec(pathname);
  return match ? { kind: match[1] as 'assistant' | 'corporate', id: match[2] } : null;
}

type View = {
  number: string;
  kind: string;
  status: 'pending' | 'approved' | 'rejected';
  fields: [string, string][];
  created: string;
  updated?: string;
  approver?: string;
  comment?: string;
  requester?: string;
  source: string;
};

const STATUS_TEXT = { pending: 'На согласовании', approved: 'Согласована', rejected: 'Отклонена' } as const;
const WORDS: Record<string, string> = { day_off: 'Отгул', vacation: 'Отпуск', read: 'Чтение', write: 'Изменение', admin: 'Администрирование' };
const dayRu = (iso?: string) => (iso ? iso.split('-').reverse().join('.') : '—');

type AssistantRequest = {
  id: string; number: string; type: string; status: string; approver?: string; decision_comment?: string; requester?: string;
  params: Record<string, string>; created_at: string; updated_at: string;
};
type CorporateRequest = { ticket_number: string; tool: string; status: string; created_at: string };
type StaffItem = { request: AssistantRequest; intern_name: string };

function fromAssistant(r: AssistantRequest): View {
  const p = r.params ?? {};
  const leave = r.type === 'leave_request';
  const fields: [string, string][] = leave
    ? [['Вид', WORDS[p.kind] ?? p.kind ?? '—'], ['С', dayRu(p.date_from)], ['По', dayRu(p.date_to)], ...(p.comment ? [['Комментарий', p.comment] as [string, string]] : [])]
    : [['Система', p.system ?? '—'], ['Ресурс', p.resource ?? '—'], ['Уровень доступа', WORDS[p.access_level] ?? p.access_level ?? '—'], ['Обоснование', p.justification ?? '—']];
  return {
    number: r.number,
    kind: leave ? 'Заявка на отсутствие' : 'Запрос доступа к системе',
    status: r.status === 'approved' ? 'approved' : r.status === 'rejected' ? 'rejected' : 'pending',
    fields, created: r.created_at, updated: r.updated_at, approver: r.approver, comment: r.decision_comment, requester: r.requester,
    source: 'Создана через AI-помощника Бади после подтверждения сотрудником',
  };
}

function fromCorporate(r: CorporateRequest): View {
  return {
    number: r.ticket_number,
    kind: 'Запрос доступа к инструменту',
    status: r.status === 'approved' ? 'approved' : r.status === 'denied' ? 'rejected' : 'pending',
    fields: [['Инструмент', r.tool]],
    created: r.created_at,
    source: 'Создана на странице корпоративных инструментов',
  };
}

const describe = (r: AssistantRequest) => fromAssistant(r).fields.map(([, v]) => v).filter(v => v !== '—').slice(0, 3).join(' · ');
const statusOf = (s: string): View['status'] => (s === 'approved' ? 'approved' : s === 'rejected' ? 'rejected' : 'pending');

type Props = { user: User; token: string; route: PortalRoute };

/** Общая шапка корпоративного портала (демо). Вики оформлена так же, см. apps/web/scripts/build-wiki.mjs. */
function Chrome({ user, children, active }: { user: User; children: React.ReactNode; active: 'requests' | 'wiki' }) {
  return (
    <div className="portal">
      <header className="portal-top">
        <span className="portal-brand"><span className="portal-logo" aria-hidden="true">▦</span> Портал сотрудника <small>демо</small></span>
        <nav aria-label="Разделы портала">
          <a className={active === 'requests' ? 'is-on' : ''} href="/portal/requests">Заявки</a>
          <a className={active === 'wiki' ? 'is-on' : ''} href="/wiki/index.html">База знаний</a>
        </nav>
        <span className="portal-user">{personName(user)}</span>
      </header>
      <main className="portal-main">{children}</main>
    </div>
  );
}

/** Список заявок: у руководителя заявки его стажёров (на согласование), у остальных собственные. */
function RequestList({ user, token }: { user: User; token: string }) {
  const lead = user.role === 'teamlead';
  const [rows, setRows] = useState<{ id: string; number: string; who: string; what: string; status: View['status'] }[] | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let cancelled = false;
    const load = lead
      ? apiRequest<StaffItem[]>('', '/api/buddy/requests', { token }).then(list => list.map(i => ({ id: i.request.id, number: i.request.number, who: i.intern_name, what: describe(i.request), status: statusOf(i.request.status) })))
      : apiRequest<AssistantRequest[]>('', '/api/requests', { token }).then(list => list.map(r => ({ id: r.id, number: r.number, who: personName(user), what: describe(r), status: statusOf(r.status) })));
    load.then(r => { if (!cancelled) setRows(r); }).catch((e: unknown) => { if (!cancelled) setError(e instanceof Error ? e.message : 'Не удалось загрузить заявки.'); });
    return () => { cancelled = true; };
  }, [lead, token, user]);
  return (
    <Chrome user={user} active="requests">
      <p className="portal-crumbs"><a href="/">AI-buddy</a> / Заявки</p>
      <article className="portal-card">
        <header><div><span className="portal-kind">{lead ? 'На согласовании у руководителя' : 'Мои заявки'}</span><h1>Заявки</h1></div></header>
        {error && <p className="portal-error" role="alert">{error}</p>}
        {!rows && !error && <p className="portal-wait">Загружаем…</p>}
        {rows && rows.length === 0 && <p className="portal-wait">Заявок пока нет.</p>}
        {rows && rows.length > 0 && (
          <ul className="portal-list">
            {rows.map(r => (
              <li key={r.id}>
                <a href={`/portal/requests/assistant/${r.id}`}><b>{r.number}</b><span>{r.who} · {r.what}</span></a>
                <span className={`portal-status is-${r.status}`}>{STATUS_TEXT[r.status]}</span>
              </li>
            ))}
          </ul>
        )}
        <p className="portal-demo">Демо-портал: данные синтетические, это не корпоративная система Авито.</p>
      </article>
    </Chrome>
  );
}

/**
 * Псевдокорпоративный сайт (демо): так выглядит «внутренний сервис», в котором живёт заявка.
 * Бади открывает его по клику на заявку в своём окне, руководитель согласует здесь заявки стажёров (с комментарием).
 */
export function PortalPage({ user, token, route }: Props) {
  if (route.kind === 'list') return <RequestList user={user} token={token} />;
  return <RequestPage user={user} token={token} route={route} />;
}

function RequestPage({ user, token, route }: { user: User; token: string; route: { kind: 'assistant' | 'corporate'; id: string } }) {
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState('');
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    const path = route.kind === 'assistant' ? `/api/requests/${route.id}` : `/api/corporate/access-requests/${route.id}`;
    apiRequest<AssistantRequest | CorporateRequest>('', path, { token })
      .then(data => { if (!cancelled) { setView(route.kind === 'assistant' ? fromAssistant(data as AssistantRequest) : fromCorporate(data as CorporateRequest)); setError(''); } })
      .catch((reason: unknown) => {
        if (cancelled) return;
        setError(reason instanceof ApiError && (reason.status === 404 || reason.status === 403)
          ? 'Заявка не найдена или принадлежит другому сотруднику.'
          : reason instanceof Error ? reason.message : 'Не удалось загрузить заявку.');
      });
    return () => { cancelled = true; };
  }, [route.kind, route.id, token]);
  useEffect(() => { setView(null); setError(''); return load(); }, [load]);

  // Решение принимает только руководитель стажёра (бэкенд проверяет роль и принадлежность)
  const canDecide = user.role === 'teamlead' && route.kind === 'assistant' && view?.status === 'pending' && !!view.requester;
  const decide = async (status: 'approved' | 'rejected') => {
    setBusy(true); setError('');
    try {
      await apiRequest('', `/api/buddy/requests/${route.id}/decision`, { token, method: 'POST', body: comment.trim() ? { status, comment: comment.trim() } : { status } });
      setComment('');
      load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось сохранить решение.');
    } finally { setBusy(false); }
  };

  const steps = view ? [
    { title: 'Заявка создана', at: view.created, note: view.source, done: true },
    { title: 'На согласовании', at: view.created, note: view.approver && view.status === 'pending' ? `Согласующий: ${view.approver}` : 'Ожидает решения руководителя', done: true },
    view.status === 'pending'
      ? { title: 'Решение', at: '', note: 'Ожидается', done: false }
      : { title: view.status === 'approved' ? 'Согласована' : 'Отклонена', at: view.updated ?? view.created, note: [view.approver, view.comment ? `Комментарий: ${view.comment}` : ''].filter(Boolean).join('. '), done: true },
  ] : [];

  return (
    <Chrome user={user} active="requests">
      <p className="portal-crumbs"><a href="/">AI-buddy</a> / <a href="/portal/requests">Заявки</a>{view ? ` / ${view.number}` : ''}</p>
      {error && <p className="portal-error" role="alert">{error}</p>}
      {!view && !error && <p className="portal-wait">Загружаем заявку…</p>}
      {view && (
        <article className="portal-card">
          <header>
            <div><span className="portal-kind">{view.kind}</span><h1>{view.number}</h1></div>
            <span className={`portal-status is-${view.status}`}>{STATUS_TEXT[view.status]}</span>
          </header>
          <dl className="portal-fields">
            <div><dt>Номер</dt><dd>{view.number}</dd></div>
            {view.requester && <div><dt>Сотрудник</dt><dd>{view.requester}</dd></div>}
            <div><dt>Создана</dt><dd>{formatDateTime(view.created)}</dd></div>
            {view.approver && view.status !== 'pending' && <div><dt>Согласующий</dt><dd>{view.approver}</dd></div>}
            {view.fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
          </dl>

          {canDecide && (
            <form className="portal-decision" onSubmit={event => event.preventDefault()}>
              <h2>Решение</h2>
              <label>Комментарий (необязательно)
                <textarea rows={3} maxLength={500} value={comment} placeholder="Например: согласовано, хорошего отдыха" onChange={event => setComment(event.target.value)} />
              </label>
              <div className="portal-decision-bar">
                <button type="button" className="portal-btn is-ok" disabled={busy} onClick={() => void decide('approved')}>Согласовать</button>
                <button type="button" className="portal-btn is-no" disabled={busy} onClick={() => void decide('rejected')}>Отклонить</button>
              </div>
            </form>
          )}

          <h2>История</h2>
          <ol className="portal-timeline">
            {steps.map(step => (
              <li key={step.title} className={step.done ? 'is-done' : ''}>
                <b>{step.title}</b>
                {step.at && <time>{formatDateTime(step.at)}</time>}
                {step.note && <span>{step.note}</span>}
              </li>
            ))}
          </ol>
          <p className="portal-demo">Демо-портал: данные синтетические, это не корпоративная система Авито.</p>
        </article>
      )}
      <a className="portal-back" href="/">← Вернуться в AI-buddy</a>
    </Chrome>
  );
}
