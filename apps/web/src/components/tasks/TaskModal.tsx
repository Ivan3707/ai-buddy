import { useEffect, useRef } from 'react';
import type { ChecklistItem, ChecklistStage } from '@ai-buddy/client';
import { History, ReviewAction } from './TaskReview';

const STATUS_LABEL: Record<ChecklistItem['status'], string> = {
  unavailable: 'Недоступно',
  active: 'Активно',
  completed: 'Завершено',
  verified: 'Проверено',
};

type Props = {
  item: ChecklistItem;
  stage: ChecklistStage;
  token: string;
  refresh: () => Promise<void>;
  busy: boolean;
  error: string;
  onComplete: () => void | Promise<void>;
  onClose: () => void;
};

function dueLabel(value: string) {
  return new Intl.DateTimeFormat('ru-RU', { dateStyle: 'long' }).format(new Date(value));
}

/**
 * Окно задачи. Задача без результата: описание и, если она активна, кнопка «Завершить».
 * Задача с результатом: форма «комментарий + ссылка» (то, что хранит бэкенд: result_text и result_url) и история отправок
 * с комментариями наставника. Загрузки файлов в бэкенде пока нет, поэтому материалы прикладывают ссылкой.
 */
export function TaskModal({ item, stage, token, refresh, busy, error, onComplete, onClose }: Props) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeRef.current?.focus();
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const weeks = `Недели ${stage.week_start}${stage.week_end !== stage.week_start ? `–${stage.week_end}` : ''}`;
  const meta = [weeks, item.estimated_minutes ? `${item.estimated_minutes} мин` : null, item.due_at ? `срок до ${dueLabel(item.due_at)}` : null]
    .filter(Boolean)
    .join(' · ');

  let action;
  if (item.status === 'unavailable') {
    action = <p className="task-modal-note">Задача откроется, когда будет выполнена текущая.</p>;
  } else if (item.requires_review) {
    if (item.status === 'active') {
      action = (
        <div className="task-modal-form legacy">
          <h3>Результат задачи</h3>
          <ReviewAction item={item} token={token} refresh={refresh} />
          <p className="task-modal-note">Файл пока приложить нельзя: добавьте ссылку на документ или материал в поле «Ссылка на результат».</p>
        </div>
      );
    } else if (item.status === 'completed') {
      action = <p className="task-modal-note">Результат отправлен. Наставник проверит его и оставит комментарий.</p>;
    } else {
      action = <p className="task-modal-note">Наставник проверил результат.</p>;
    }
  } else if (item.status === 'active') {
    action = (
      <p className="task-modal-note">Для этой задачи результат не нужен: когда выполните её, нажмите «Завершить».</p>
    );
  } else {
    action = <p className="task-modal-note">Задача выполнена.</p>;
  }

  return (
    <div className="task-modal-backdrop" onClick={onClose}>
      <section className="task-modal" role="dialog" aria-modal="true" aria-labelledby="task-modal-title" onClick={event => event.stopPropagation()}>
        <header className={`task-modal-head is-${item.status}`}>
          <div>
            <span className="task-modal-meta">{meta}</span>
            <h2 id="task-modal-title">{item.title}</h2>
          </div>
          <span className="task-modal-status">{STATUS_LABEL[item.status]}</span>
        </header>
        <div className="task-modal-body">
          {item.description && (
            <section>
              <h3>Описание</h3>
              <p>{item.description}</p>
            </section>
          )}
          {item.expected_result && (
            <section>
              <h3>Ожидаемый результат</h3>
              <p>{item.expected_result}</p>
            </section>
          )}
          {error && <p className="task-modal-error" role="alert">{error}</p>}
          {action}
          {item.requires_review && item.status !== 'unavailable' && <History itemID={item.id} token={token} />}
        </div>
        <footer className="task-modal-foot">
          <button type="button" ref={closeRef} className="task-modal-close" onClick={onClose}>Закрыть</button>
          {item.status === 'active' && !item.requires_review && (
            <button type="button" className="task-modal-complete" disabled={busy} onClick={() => void onComplete()}>
              {busy ? 'Сохраняем…' : 'Завершить'}
            </button>
          )}
        </footer>
      </section>
    </div>
  );
}
