import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { getInternSubmissionHistory, getSubmissionHistory, submitItem, type ChecklistItem, type SubmissionHistory } from '@ai-buddy/client';
import { InternHistory } from '../staff/InternHistory';

function dateTime(value: string) { return new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)); }
const errorText = (reason: unknown) => (reason instanceof Error ? reason.message : 'Не удалось загрузить историю');

export function ReviewAction({ item, token, refresh }: { item: ChecklistItem; token: string; refresh: () => Promise<void> }) {
  const [text, setText] = useState(''); const [url, setUrl] = useState(''); const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); try { await submitItem('', token, item.id, { result_text: text.trim() || null, result_url: url.trim() || null }); setText(''); setUrl(''); await refresh(); } catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось отправить результат'); } finally { setBusy(false); } };
  return <form className="submission-form" onSubmit={submit}><label>Комментарий к результату<textarea value={text} onChange={e => setText(e.target.value)} placeholder="Опишите, что сделано"/></label><label>Ссылка на результат<input type="url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://…"/></label>{error && <p className="notice error">{error}</p>}<button disabled={busy || (!text.trim() && !url.trim())}>{busy ? 'Отправляем…' : 'Отправить на проверку'}</button></form>;
}

/**
 * История отправок стажёра в окне задачи. Загружается сразу, чтобы знать, есть ли что показать:
 * пока отправок нет, блока нет вовсе. Если история не загрузилась, ссылка остаётся и при раскрытии повторяет запрос.
 */
export function History({ itemID, token }: { itemID: string; token: string }) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<SubmissionHistory[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    setBusy(true); setError('');
    try { setItems((await getSubmissionHistory('', token, itemID)) ?? []); } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); }
  }, [token, itemID]);
  useEffect(() => { setItems(null); setOpen(false); void load(); }, [load]);

  if (!error && (!items || items.length === 0)) return null;
  const toggle = () => { if (open) { setOpen(false); return; } setOpen(true); if (error) void load(); };
  return <div className="history"><button className="link-button" onClick={toggle}>{open ? 'Скрыть историю' : 'История отправок'}</button>{open && <div>{busy ? <p className="muted">Загрузка…</p> : error ? <p className="notice error">{error}</p> : items?.map(entry => <div className="history-entry" key={entry.id}><div><b>Попытка {entry.attempt_number}</b><span className={`status ${entry.status}`}>{entry.status === 'pending' ? 'На проверке' : entry.status === 'approved' ? 'Одобрено' : 'Отклонено'}</span></div><small>{dateTime(entry.submitted_at)}</small>{entry.result_text && <p>{entry.result_text}</p>}{entry.result_url && <a href={entry.result_url} target="_blank" rel="noreferrer">Открыть результат ↗</a>}{entry.review_comment && <p className="review-comment"><b>Комментарий buddy:</b> {entry.review_comment}</p>}</div>)}</div>}</div>;
}

/**
 * История отправок для наставника и руководителя в окне задачи: блок появляется, только когда у задачи есть отправки
 * (сначала короткая проверка, затем общий блок InternHistory). При ошибке проверки блок тоже показывается — он сообщит об ошибке сам.
 */
export function StaffHistory({ internId, itemId, token }: { internId: string; itemId: string; token: string }) {
  const [show, setShow] = useState(false);
  useEffect(() => {
    let alive = true;
    setShow(false);
    Promise.resolve(getInternSubmissionHistory('', token, internId, itemId))
      .then(list => { if (alive) setShow((list ?? []).length > 0); })
      .catch(() => { if (alive) setShow(true); });
    return () => { alive = false; };
  }, [token, internId, itemId]);
  return show ? <InternHistory internId={internId} itemId={itemId} token={token} /> : null;
}
