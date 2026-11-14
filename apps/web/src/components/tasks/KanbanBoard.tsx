import { useState } from 'react';
import { completeItem, type Checklist, type ChecklistItem } from '@ai-buddy/client';
import { History, ReviewAction } from '../ChecklistView';

type Status = ChecklistItem['status'];

/** Колонки доски: порядок и подписи заданы дизайнером. */
export const KANBAN_COLUMNS: { status: Status; title: string }[] = [
  { status: 'unavailable', title: 'Недоступно' },
  { status: 'active', title: 'Активно' },
  { status: 'completed', title: 'Завершено' },
  { status: 'verified', title: 'Проверено' },
];

type Props = { checklist: Checklist; token: string; refresh: () => Promise<void> };

/** Задачи одного чек-листа в виде канбан-доски по статусам (набросок, макет доски ещё не готов). */
export function KanbanBoard({ checklist, token, refresh }: Props) {
  const [busyID, setBusyID] = useState('');
  const [error, setError] = useState('');
  const trackName = checklist.type === 'common' ? 'Общий' : 'Профессиональный';
  const cards = checklist.stages.flatMap(stage => stage.items.map(item => ({ item, stage })));

  const complete = async (item: ChecklistItem) => {
    setBusyID(item.id);
    setError('');
    try {
      await completeItem('', token, item.id);
      await refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось завершить задачу');
    } finally {
      setBusyID('');
    }
  };

  return (
    <section className="kanban" aria-label={`${trackName} чек-лист`}>
      {error && <p className="notice error" role="alert">{error}</p>}
      <div className="kanban-columns">
        {KANBAN_COLUMNS.map(column => {
          const inColumn = cards.filter(card => card.item.status === column.status);
          return (
            <section key={column.status} className={`kanban-column is-${column.status}`} aria-label={column.title}>
              <header className="kanban-column-head">
                <h3>{column.title}</h3>
                <span>{inColumn.length}</span>
              </header>
              <div className="kanban-cards">
                {inColumn.length === 0 && <p className="kanban-empty">Пусто</p>}
                {inColumn.map(({ item, stage }) => (
                  <article key={item.id} className={`kanban-card is-${item.status}`}>
                    <span className="kanban-card-stage">
                      Недели {stage.week_start}{stage.week_end !== stage.week_start && `–${stage.week_end}`}
                    </span>
                    <h4>{item.title}</h4>
                    {item.description && <p>{item.description}</p>}
                    {item.estimated_minutes && <span className="kanban-card-time">≈ {item.estimated_minutes} мин</span>}
                    {item.status === 'active' && !item.requires_review && (
                      <button disabled={busyID === item.id} onClick={() => complete(item)}>
                        {busyID === item.id ? 'Сохраняем…' : 'Завершить'}
                      </button>
                    )}
                    {item.status === 'active' && item.requires_review && <ReviewAction item={item} token={token} refresh={refresh} />}
                    {item.requires_review && <History itemID={item.id} token={token} />}
                  </article>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </section>
  );
}
