import { useEffect, useState } from 'react';
import { ApiError, apiRequest, type User } from '@ai-buddy/client';
import { formatDateTime, personName } from '../lib/format';

export type PortalRoute = { kind: 'assistant' | 'corporate'; id: string };

/** /portal/requests/<kind>/<id> -> маршрут, иначе null. */
export function parsePortalPath(pathname: string): PortalRoute | null {
  const match = /^\/portal\/requests\/(assistant|corporate)\/([\w-]{8,64})\/?$/.exec(pathname);
  return match ? { kind: match[1] as PortalRoute['kind'], id: match[2] } : null;
}

type View = {
  number: string;
  kind: string;
  status: 'pending' | 'approved' | 'rejected';
  fields: [string, string][];
  created: string;
  updated?: string;
  approver?: string;
  source: string;
};

const STATUS_TEXT = { pending: 'На согласовании', approved: 'Согласована', rejected: 'Отклонена' } as const;
const WORDS: Record<string, string> = { day_off: 'Отгул', vacation: 'Отпуск', read: 'Чтение', write: 'Изменение', admin: 'Администрирование' };
const dayRu = (iso?: string) => (iso ? iso.split('-').reverse().join('.') : '—');

type AssistantRequest = { number: string; type: string; status: string; approver?: string; params: Record<string, string>; created_at: string; updated_at: string };
type CorporateRequest = { ticket_number: string; tool: string; status: string; created_at: string };

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
    fields, created: r.created_at, updated: r.updated_at, approver: r.approver,
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

type Props = { user: User; token: string; route: PortalRoute };

/**
 * Псевдокорпоративный сайт (демо): так выглядит «внутренний сервис», в котором живёт заявка.
 * Бади открывает его по клику на заявку в своём окне. Данные те же, что в API, оформление намеренно другое.
 */
export function PortalPage({ user, token, route }: Props) {
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    setView(null); setError('');
    const path = route.kind === 'assistant' ? `/api/requests/${route.id}` : `/api/corporate/access-requests/${route.id}`;
    apiRequest<AssistantRequest | CorporateRequest>('', path, { token })
      .then(data => { if (!cancelled) setView(route.kind === 'assistant' ? fromAssistant(data as AssistantRequest) : fromCorporate(data as CorporateRequest)); })
      .catch((reason: unknown) => {
        if (cancelled) return;
        setError(reason instanceof ApiError && (reason.status === 404 || reason.status === 403)
          ? 'Заявка не найдена или принадлежит другому сотруднику.'
          : reason instanceof Error ? reason.message : 'Не удалось загрузить заявку.');
      });
    return () => { cancelled = true; };
  }, [route.kind, route.id, token]);

  const steps = view ? [
    { title: 'Заявка создана', at: view.created, note: view.source, done: true },
    { title: 'На согласовании', at: view.created, note: view.approver ? `Согласующий: ${view.approver}` : 'Ожидает решения согласующего', done: true },
    view.status === 'pending'
      ? { title: 'Решение', at: '', note: 'Ожидается', done: false }
      : { title: view.status === 'approved' ? 'Согласована' : 'Отклонена', at: view.updated ?? view.created, note: view.approver ?? '', done: true },
  ] : [];

  return (
    <div className="portal">
      <header className="portal-top">
        <span className="portal-brand"><span className="portal-logo" aria-hidden="true">▦</span> Портал сотрудника <small>демо</small></span>
        <nav aria-label="Разделы портала">
          <span className="is-on">Мои заявки</span>
          <a href="/wiki/index.html" target="_blank" rel="noreferrer">База знаний</a>
        </nav>
        <span className="portal-user">{personName(user)}</span>
      </header>

      <main className="portal-main">
        <p className="portal-crumbs"><a href="/">AI-buddy</a> / Мои заявки{view ? ` / ${view.number}` : ''}</p>
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
              <div><dt>Создана</dt><dd>{formatDateTime(view.created)}</dd></div>
              {view.approver && <div><dt>Согласующий</dt><dd>{view.approver}</dd></div>}
              {view.fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
            </dl>
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
      </main>
    </div>
  );
}
