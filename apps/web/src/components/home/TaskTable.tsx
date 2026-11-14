import type { Checklist } from '@ai-buddy/client';
import { currentStage, isDone, weekLabel } from '../../lib/format';
import { MANAGER_NAME } from '../../lib/stubs';
import { CheckIcon, LockIcon } from '../icons';

type Props = { checklist: Checklist; onOpen: () => void };

/** Задачи текущего этапа выбранного чек-листа. */
export function TaskTable({ checklist, onOpen }: Props) {
  const stage = currentStage(checklist);
  const items = stage?.items ?? [];
  const done = items.filter(isDone).length;
  return (
    <section className="task-table" aria-label="Задачи текущего этапа">
      <header>
        <h2>Задачи</h2>
        <strong>{done}/{items.length} выполнено</strong>
      </header>
      <div className="task-columns" aria-hidden="true">
        <span>Задачи</span><span>Руководитель</span><span>Срок</span><span>Комментарий</span>
      </div>
      {items.length === 0 && <p className="task-empty">В этом чек-листе пока нет задач.</p>}
      {items.map(item => {
        const locked = item.status === 'unavailable';
        const mark = locked
          ? <LockIcon width={16} height={16} />
          : isDone(item) ? <CheckIcon width={16} height={16} /> : null;
        return (
          <article
            key={item.id}
            className={`task-row${locked ? ' is-locked' : ''}${isDone(item) ? ' is-done' : ''}`}
            onClick={onOpen}
          >
            <span className="task-name"><span className="task-mark">{mark}</span>{item.title}</span>
            <span className="task-cell">{locked ? 'Недоступно' : MANAGER_NAME}</span>
            <span className="task-cell">{locked || !stage ? 'Недоступно' : weekLabel(stage)}</span>
            <span className="task-cell task-comment">{locked ? 'Недоступно' : item.description}</span>
          </article>
        );
      })}
    </section>
  );
}
