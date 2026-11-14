import type { ChecklistsState } from '../hooks/useChecklists';
import { KanbanBoard } from '../components/tasks/KanbanBoard';

type Kind = 'common' | 'professional';
type Props = { token: string; checklists: ChecklistsState; selected?: Kind; onSelect?: (kind: Kind) => void };

const TABS: { kind: Kind; title: string; lines: [string, string] }[] = [
  { kind: 'common', title: 'Общий чек-лист', lines: ['Общий', 'чек-лист'] },
  { kind: 'professional', title: 'Профессиональный чек-лист', lines: ['Профессиональный', 'чек-лист'] },
];

/** Задачи: белая панель с выбором чек-листа (набросок по макету check.png); внутри панели — канбан-доска по статусам. */
export function TasksPage({ token, checklists, selected = 'common', onSelect }: Props) {
  const { data, loading, error, refresh } = checklists;
  const current = TABS.find(tab => tab.kind === selected) ?? TABS[0];
  return (
    <div className="tasks-page">
      <h1 className="visually-hidden">Задачи</h1>
      <div className="tasks-title"><h2>{current.title}</h2></div>
      <div className="tasks-switch" role="group" aria-label="Выбор чек-листа">
        {TABS.map(tab => (
          <button
            key={tab.kind}
            className={`tasks-switch-btn${tab.kind === selected ? ' is-selected' : ''}`}
            aria-pressed={tab.kind === selected}
            onClick={() => onSelect?.(tab.kind)}
          >
            <span>{tab.lines[0]}</span><span>{tab.lines[1]}</span>
          </button>
        ))}
      </div>
      <div className="tasks-panel legacy">
        {loading && !data && <p className="state-message">Загружаем задачи…</p>}
        {error && <p className="state-message state-error" role="alert">{error}</p>}
        {data && <KanbanBoard key={selected} checklist={data[selected]} token={token} refresh={refresh} />}
      </div>
    </div>
  );
}
