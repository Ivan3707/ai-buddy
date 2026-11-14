import type { ChecklistsState } from '../hooks/useChecklists';
import { ChecklistView } from '../components/ChecklistView';

type Props = { token: string; checklists: ChecklistsState };

/** Полный список задач обоих чек-листов. Оформление по макету — следующий шаг. */
export function TasksPage({ token, checklists }: Props) {
  const { data, loading, error, refresh } = checklists;
  return (
    <div className="tasks-page legacy">
      <h1 className="page-heading">Задачи</h1>
      {loading && !data && <p className="state-message">Загружаем задачи…</p>}
      {error && <p className="state-message state-error" role="alert">{error}</p>}
      {data && (
        <div className="checklist-grid">
          <ChecklistView checklist={data.common} token={token} refresh={refresh} />
          <ChecklistView checklist={data.professional} token={token} refresh={refresh} />
        </div>
      )}
    </div>
  );
}
