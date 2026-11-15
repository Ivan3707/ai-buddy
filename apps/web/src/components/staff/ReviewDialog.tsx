import { useEffect, useState } from 'react';
import { getInternProgress, type ChecklistItem, type ChecklistStage, type PendingSubmission } from '@ai-buddy/client';
import { formatDateTime } from '../../lib/format';
import { Dialog } from './Dialog';
import { InternHistory } from './InternHistory';

type Props = {
  submission: PendingSubmission;
  internName: string;
  token: string;
  error?: string;
  onDecide: (approved: boolean, comment: string) => void;
  onClose: () => void;
};

type Context = { item: ChecklistItem; stage: ChecklistStage; checklist: string };

/** Окно проверки: что за задача, что требовалось, кто и что сдал. Без контекста наставнику нечего сравнивать с результатом. */
export function ReviewDialog({ submission, internName, token, error, onDecide, onClose }: Props) {
  const [context, setContext] = useState<Context | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [comment, setComment] = useState('');

  useEffect(() => {
    let alive = true;
    setLoaded(false);
    getInternProgress('', token, submission.intern_id)
      .then(progress => {
        if (!alive) return;
        const lists = [
          ...(progress.common ? [{ name: 'Общий', list: progress.common }] : []),
          { name: 'Профессиональный', list: progress.professional },
        ];
        for (const { name, list } of lists) {
          for (const stage of list.stages) {
            const item = stage.items.find(entry => entry.id === submission.item_id);
            if (item) { setContext({ item, stage, checklist: name }); return; }
          }
        }
      })
      .catch(() => undefined)
      .finally(() => { if (alive) setLoaded(true); });
    return () => { alive = false; };
  }, [token, submission.intern_id, submission.item_id]);

  const item = context?.item;
  return (
    <Dialog title={`Проверка: ${submission.item_title}`} onClose={onClose}>
      <section className="review-block" aria-label="Задача">
        <h3>Задача</h3>
        <p><b>{submission.item_title}</b></p>
        <p className="muted">Стажёр: {internName}{context ? ` · ${context.checklist} чек-лист · ${context.stage.title}` : ''}</p>
        {!loaded && <p className="muted">Загружаем описание задачи…</p>}
        {loaded && !item && <p className="muted">Описание задачи недоступно.</p>}
        {item && (
          <>
            <p className="review-text">{item.description || 'Описание не задано.'}</p>
            {item.expected_result && <p className="review-text"><b>Что нужно сделать (ожидаемый результат):</b> {item.expected_result}</p>}
          </>
        )}
      </section>

      <section className="review-block" aria-label="Текущая отправка">
        <h3>Отправка на проверке</h3>
        <p><b>Попытка {submission.attempt_number}</b> · {formatDateTime(submission.submitted_at)} · на проверке</p>
        <p className="review-result"><b>Результат:</b> {submission.result_text || (submission.result_url ? '' : 'Не указан')}
          {submission.result_url && <> <a href={submission.result_url} target="_blank" rel="noreferrer">{submission.result_url}</a></>}</p>
      </section>
      {submission.attempt_number > 1 && <InternHistory internId={submission.intern_id} itemId={submission.item_id} token={token} heading="Предыдущие попытки и ревью" />}

      <label className="field">Комментарий при возврате<textarea value={comment} onChange={event => setComment(event.target.value)} /></label>
      {error && <p className="state-message state-error" role="alert">{error}</p>}
      <div className="dialog-actions">
        <button className="pill pill-dark pill-sm" onClick={() => onDecide(false, comment)}>Вернуть</button>
        <button className="pill pill-black pill-sm" onClick={() => onDecide(true, comment)}>Принять</button>
      </div>
    </Dialog>
  );
}
