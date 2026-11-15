import type { Checklist, ChecklistItem, ChecklistStage } from '@ai-buddy/client';
import { isDone, weekLabel } from '../../lib/format';
import { MANAGER_NAME } from '../../lib/stubs';
import { CheckIcon, LockIcon } from '../icons';

type Props = { checklist: Checklist; onOpen: (item: ChecklistItem) => void };

/** Порядок в списке: сверху активные, ниже на проверке, затем недоступные, в самом низу проверенные. */
const STATUS_ORDER: Record<ChecklistItem['status'], number> = { active: 0, completed: 1, unavailable: 2, verified: 3 };

/** Все задачи выбранного чек-листа одним прокручиваемым списком. */
export function TaskTable({ checklist, onOpen }: Props) {
  const rows = checklist.stages
    .flatMap((stage: ChecklistStage) => stage.items.map(item => ({ item, stage })))
    .map((row, index) => ({ ...row, index }))
    .sort((a, b) => STATUS_ORDER[a.item.status] - STATUS_ORDER[b.item.status] || a.index - b.index);
  const done = rows.filter(row => isDone(row.item)).length;
  return (
    <section className="task-table" aria-label="Задачи чек-листа">
      <header>
        <h2>Задачи</h2>
        <strong>{done}/{rows.length} выполнено</strong>
      </header>
      <div className="task-columns" aria-hidden="true">
        <span>Задачи</span><span>Руководитель</span><span>Срок</span><span>Описание</span>
      </div>
      <div className="task-rows">
      {rows.length === 0 && <p className="task-empty">В этом чек-листе пока нет задач.</p>}
      {rows.map(({ item, stage }) => {
        const locked = item.status === 'unavailable';
        const mark = locked ? <LockIcon /> : <CheckIcon className="task-check" />;
        return (
          <article
            key={item.id}
            className={`task-row${locked ? ' is-locked' : ''}${isDone(item) ? ' is-done' : ''}`}
          >
            <span
              className="task-name"
              role={locked ? undefined : 'button'}
              tabIndex={locked ? undefined : 0}
              onClick={locked ? undefined : () => onOpen(item)}
              onKeyDown={locked ? undefined : event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onOpen(item); } }}
            ><span className="task-mark">{mark}</span>{item.title}</span>
            <span className="task-cell">{locked ? 'Недоступно' : MANAGER_NAME}</span>
            <span className="task-cell">{locked ? 'Недоступно' : weekLabel(stage)}</span>
            <span className="task-cell task-comment">{locked ? 'Недоступно' : item.description}</span>
          </article>
        );
      })}
      </div>
    </section>
  );
}
