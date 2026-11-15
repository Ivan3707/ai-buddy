import { useEffect, useState } from 'react';
import { getInternSubmissionHistory, type SubmissionHistory } from '@ai-buddy/client';
import { formatDateTime } from '../../lib/format';

const STATUS: Record<string, string> = { pending: 'На проверке', approved: 'Принято', rejected: 'Возвращено' };

/** История отправок задачи стажёра (от старой к новой), только чтение. Видна и наставнику, и руководителю. */
export function InternHistory({ internId, itemId, token, heading = 'История отправок' }: { internId: string; itemId: string; token: string; heading?: string }) {
  const [items, setItems] = useState<SubmissionHistory[] | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let alive = true;
    setItems(null); setError('');
    Promise.resolve(getInternSubmissionHistory('', token, internId, itemId))
      .then(list => { if (alive) setItems(list ?? []); })
      .catch(reason => { if (alive) setError(reason instanceof Error ? reason.message : 'Не удалось загрузить историю'); });
    return () => { alive = false; };
  }, [token, internId, itemId]);
  return (
    <section className="review-block history-list" aria-label={heading}>
      <h3>{heading}</h3>
      {error && <p className="state-message state-error" role="alert">{error}</p>}
      {!items && !error && <p className="muted">Загружаем историю…</p>}
      {items && items.length === 0 && <p className="muted">Отправок пока нет.</p>}
      {items?.map(entry => (
        <div key={entry.id} className="history-item">
          <p><b>Попытка {entry.attempt_number}</b> · {formatDateTime(entry.submitted_at)} · {STATUS[entry.status] ?? entry.status}</p>
          {(entry.result_text || entry.result_url) && (
            <p className="review-result"><b>Результат:</b> {entry.result_text}
              {entry.result_url && <> <a href={entry.result_url} target="_blank" rel="noreferrer">{entry.result_url}</a></>}</p>
          )}
          {entry.review_comment && <p className="review-text"><b>Комментарий проверяющего:</b> {entry.review_comment}</p>}
        </div>
      ))}
    </section>
  );
}
