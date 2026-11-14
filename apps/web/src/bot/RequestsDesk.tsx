import { useCallback, useEffect, useState } from 'react';
import { bot, REQUEST_STATUS, type ServiceRequest } from './api';

// Teamlead side: the interns' requests. "Согласовать" opens the request in the corporate portal (mock), where the decision
// and the comment are made. Buddies do not approve requests.

const WORDS: Record<string, string> = { day_off: 'отгул', vacation: 'отпуск', read: 'чтение', write: 'запись', admin: 'админ' };
const describe = (r: ServiceRequest) => r.type === 'leave_request'
  ? `${WORDS[r.params.kind] ?? r.params.kind}: ${r.params.date_from}${r.params.date_to !== r.params.date_from ? ` — ${r.params.date_to}` : ''}`
  : `${r.params.system} · ${r.params.resource} · ${WORDS[r.params.access_level] ?? r.params.access_level}`;

export function RequestsDesk({ token }: { token: string }) {
  const [items, setItems] = useState<{ request: ServiceRequest; intern_name: string }[]>([]);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    try { setItems(await bot.staffRequests(token)); setError(''); }
    catch (e) { setError(e instanceof Error ? e.message : 'Ошибка'); }
  }, [token]);
  useEffect(() => { void load(); const id = setInterval(() => void load(), 15000); return () => clearInterval(id); }, [load]);
  const pending = items.filter(i => i.request.status === 'pending_approval').length;

  return <div className="desk">
    {error && <p className="notice error">{error}</p>}
    <section className="staff-card">
      <span className="eyebrow">Корпоративные сервисы (демо)</span>
      <h2>Заявки стажёров {pending > 0 && <span className="count-badge">{pending}</span>}</h2>
      {items.length ? items.map(({ request: r, intern_name }) => <div className="desk-request" key={r.id}>
        <div><b>{r.number}</b> · {intern_name || 'Стажёр'}<br /><small className="muted">{describe(r)}{r.params.justification ? ` · ${r.params.justification}` : ''}</small>
          {r.decision_comment && <><br /><small className="muted">Комментарий: {r.decision_comment}</small></>}</div>
        {r.status === 'pending_approval'
          ? <a className="pill pill-black pill-sm" href={`/portal/requests/assistant/${r.id}`} target="_blank" rel="noreferrer">Согласовать</a>
          : <a className={`chip ${r.status === 'approved' ? 'completed' : ''}`} href={`/portal/requests/assistant/${r.id}`} target="_blank" rel="noreferrer">{REQUEST_STATUS[r.status]}</a>}
      </div>) : <p className="muted">Заявок пока нет. Стажёр оформляет их в чате с ботом.</p>}
    </section>
  </div>;
}
