import { useEffect, useState, type FormEvent } from 'react';
import { ApiError, getFeedback, submitFeedback, type Feedback, type User } from '@ai-buddy/client';
import type { ChecklistsState } from '../hooks/useChecklists';
import { displayName } from '../lib/format';

type Props = { user: User; token: string; checklists: ChecklistsState };

const DELIVERY: Record<Feedback['delivery_status'], string> = {
  pending: 'Передаём в аналитику…',
  sent: 'Передано в аналитику',
  failed: 'Передать пока не удалось, попробуем ещё раз',
};
const BACKEND_ERRORS: Record<string, string> = {
  'checklist is not fully completed and reviewed': 'Отзыв можно оставить, когда оба чек-листа завершены и проверены.',
  'feedback already submitted': 'Вы уже отправляли отзыв, спасибо!',
  'rating must be 1..5 and comment at most 4000 characters': 'Оценка от 1 до 5, комментарий не длиннее 4000 символов.',
};
const errorText = (reason: unknown) => (reason instanceof Error ? BACKEND_ERRORS[reason.message] ?? reason.message : 'Не удалось отправить');

/** Обратная связь после чек-листов: оценка и комментарий уходят в бэкенд по каждому из двух чек-листов, там же видно статус передачи. */
export function FeedbackPage({ user, token, checklists }: Props) {
  const [rating, setRating] = useState(0);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [sent, setSent] = useState<Feedback[]>([]);
  const lists = checklists.data ? [checklists.data.common, checklists.data.professional] : [];
  const ids = lists.map(list => list.id).join(',');

  // отзыв мог быть отправлен раньше: показываем спасибо и статус, а не форму
  useEffect(() => {
    if (!ids) return;
    let cancelled = false;
    Promise.all(ids.split(',').map(id => getFeedback('', token, id)))
      .then(results => { if (!cancelled) setSent(results.flatMap(result => (result.feedback ? [result.feedback] : []))); })
      .catch(() => { /* форма остаётся: ошибка будет видна при отправке */ });
    return () => { cancelled = true; };
  }, [ids, token]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (rating < 1) { setError('Поставьте оценку от 1 до 5'); return; }
    setBusy(true);
    setError('');
    try {
      for (const id of ids.split(',')) {
        try {
          await submitFeedback('', token, id, { rating, comment: message.trim() });
        } catch (reason) {
          // отзыв по этому чек-листу уже есть: это не ошибка
          if (!(reason instanceof ApiError && reason.status === 409 && reason.message === 'feedback already submitted')) throw reason;
        }
      }
      const results = await Promise.all(ids.split(',').map(id => getFeedback('', token, id)));
      setSent(results.flatMap(result => (result.feedback ? [result.feedback] : [])));
      setMessage('');
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setBusy(false);
    }
  };

  const done = ids !== '' && sent.length >= ids.split(',').length;
  return (
    <main className="feedback">
      <h1>Есть идеи? Мы открыты к вашим предложениям</h1>
      <p className="feedback-lead">Расскажи больше о себе и что у тебя на уме</p>
      {done ? (
        <div className="feedback-done" role="status">
          <p className="feedback-note">Спасибо! Отзыв отправлен.</p>
          {[...new Set(sent.map(item => `Оценка ${item.rating} из 5 · ${DELIVERY[item.delivery_status]}`))].map(line => <p key={line} className="feedback-delivery">{line}</p>)}
        </div>
      ) : (
        <form onSubmit={submit}>
          <label>Имя<input value={displayName(user.email)} readOnly /></label>
          <label>Почта<input type="email" value={user.email} readOnly /></label>
          <div className="rating-field" role="group" aria-label="Оценка адаптации">
            <span>Как вам адаптация?</span>
            <div className="rating">
              {[1, 2, 3, 4, 5].map(value => (
                <button key={value} type="button" aria-label={`Оценка ${value} из 5`} aria-pressed={rating === value} onClick={() => setRating(value)}>{value}</button>
              ))}
            </div>
          </div>
          <label>
            Опиши свою идею или задай вопрос
            <textarea maxLength={4000} value={message} onChange={event => setMessage(event.target.value)} />
          </label>
          {error && <p className="feedback-note feedback-error" role="alert">{error}</p>}
          <button className="pill pill-black" disabled={busy}>{busy ? 'Отправляем…' : 'Отправить'}</button>
        </form>
      )}
    </main>
  );
}
