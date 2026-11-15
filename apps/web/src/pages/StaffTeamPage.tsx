import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  adminCompleteChecklists, ApiError, createPersonalTask, getAssignedInterns, getInternProgress, getPendingSubmissions,
  removeInternTask, reviewSubmission, updateInternTask,
  type ChecklistItem, type ChecklistType, type PendingSubmission, type ProgressResponse, type User,
} from '@ai-buddy/client';
import { ProgressRing } from '../components/home/ProgressRing';
import { Dialog } from '../components/staff/Dialog';
import { ReviewDialog } from '../components/staff/ReviewDialog';
import { TaskModal } from '../components/tasks/TaskModal';
import { formatDateTime, personName, professionLabel } from '../lib/format';
import { columnOf, statusLabel } from '../lib/taskColumns';

export type TeamTab = 'interns' | 'review';
type Props = { user: User; token: string; initialTab?: TeamTab; initialIntern?: string };

const EMPTY_TASK = { checklist_type: 'professional' as ChecklistType,  title: '', description: '', expected_result: '', due_at: '', position: 1, estimated_minutes: 30, requires_review: false };
const message = (reason: unknown) => {
  if (reason instanceof ApiError) {
    if (reason.status === 404) return 'Задача или стажёр не найдены: возможно, задачу уже удалили. Обновите страницу.';
    if (reason.status === 409) return 'Конфликт: задачу нельзя изменить в текущем состоянии (например, она уже убрана из чек-листа).';
    if (reason.status === 403) return 'Недостаточно прав для этого действия.';
  }
  return reason instanceof Error ? reason.message : 'Ошибка';
};
const toLocalInput = (iso: string | null) => {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};
type EditForm = { id: string; title: string; description: string; expected_result: string; estimated_minutes: number; due_at: string };

/** Команда: стажёры, их прогресс, очередь проверок наставника и добавление задач руководителем. */
export function StaffTeamPage({ user, token, initialTab, initialIntern }: Props) {
  const [interns, setInterns] = useState<User[]>([]);
  const [selected, setSelected] = useState(initialIntern ?? '');
  const [progress, setProgress] = useState<ProgressResponse>();
  const [error, setError] = useState('');
  const [taskOpen, setTaskOpen] = useState(false);
  const [task, setTask] = useState(EMPTY_TASK);
  const [pending, setPending] = useState<PendingSubmission[]>([]);
  const [review, setReview] = useState<PendingSubmission | null>(null);
  const [tab, setTab] = useState<TeamTab>(initialTab === 'review' ? 'review' : 'interns');
  const [openTask, setOpenTask] = useState('');
  const [edit, setEdit] = useState<EditForm | null>(null);
  const [removing, setRemoving] = useState<ChecklistItem | null>(null);
  const [demoNote, setDemoNote] = useState('');
  const [demoBusy, setDemoBusy] = useState(false);
  const isBuddy = user.role === 'buddy';
  const isLead = user.role === 'teamlead';

  const loadInterns = useCallback(async () => {
    try {
      const list = await getAssignedInterns('', token);
      setInterns(list);
      setSelected(current => current || list[0]?.id || '');
    } catch (reason) { setError(message(reason)); }
  }, [token]);
  const loadPending = useCallback(async () => {
    if (!isBuddy) return;
    try { setPending(await getPendingSubmissions('', token)); } catch (reason) { setError(message(reason)); }
  }, [token, isBuddy]);
  const loadProgress = useCallback(async (id: string) => {
    try { setProgress(await getInternProgress('', token, id)); } catch (reason) { setError(message(reason)); }
  }, [token]);

  useEffect(() => { void loadInterns(); void loadPending(); }, [loadInterns, loadPending]);
  useEffect(() => { if (selected) void loadProgress(selected); }, [selected, loadProgress]);

  const internName = (id: string) => { const found = interns.find(entry => entry.id === id); return found ? personName(found) : 'Стажёр'; };
  const totalOf = (type: ChecklistType) => (type === 'common' ? progress?.common?.total_items : progress?.professional.total_items) ?? 0;
  const addTask = async (event: FormEvent) => {
    event.preventDefault();
    if (!selected) return;
    try {
      await createPersonalTask('', token, selected, { ...task, due_at: task.due_at ? new Date(task.due_at).toISOString() : null });
      await loadProgress(selected);
      setTaskOpen(false);
      setError('');
    } catch (reason) { setError(message(reason)); }
  };
  const saveEdit = async (event: FormEvent) => {
    event.preventDefault();
    if (!edit || !selected) return;
    try {
      await updateInternTask('', token, selected, edit.id, {
        title: edit.title.trim(), description: edit.description, expected_result: edit.expected_result,
        estimated_minutes: edit.estimated_minutes, ...(edit.due_at ? { due_at: new Date(edit.due_at).toISOString() } : {}),
      });
      setEdit(null); setError('');
      await loadProgress(selected);
    } catch (reason) { setError(message(reason)); }
  };
  const confirmRemove = async () => {
    if (!removing || !selected) return;
    try {
      await removeInternTask('', token, selected, removing.id);
      setRemoving(null); setError('');
      await loadProgress(selected);
    } catch (reason) { setError(message(reason)); setRemoving(null); }
  };
  const completeDemo = async () => {
    if (!selected || demoBusy) return;
    setDemoBusy(true); setDemoNote('');
    try {
      const result = await adminCompleteChecklists('', token, selected);
      setDemoNote(`Демо: завершено задач ${result.items_completed}, принято отправок ${result.submissions_approved}.`);
      setError('');
      await loadProgress(selected);
    } catch (reason) { setError(message(reason)); }
    finally { setDemoBusy(false); }
  };
  const decide = async (approved: boolean, comment: string) => {
    if (!review) return;
    if (!approved && !comment.trim()) { setError('Для возврата нужен комментарий'); return; }
    try {
      await reviewSubmission('', token, review.id, { approved, comment: approved ? undefined : comment.trim() });
      setReview(null); setError('');
      await loadPending();
      if (selected) await loadProgress(selected);
    } catch (reason) { setError(message(reason)); }
  };

  const lists = progress ? [
    ...(progress.common ? [{ title: 'Общий чек-лист', checklist: progress.common }] : []),
    { title: 'Профессиональный чек-лист', checklist: progress.professional },
  ] : [];

  return (
    <div className="home staff-team">
      <header className="greeting">
        <h1>Команда</h1>
        <p>{isBuddy ? 'Ваши стажёры, их прогресс и работы на проверку' : 'Стажёры вашей команды и их прогресс'}</p>
      </header>
      {error && <p className="state-message state-error" role="alert">{error}</p>}

      {isBuddy && (
        <div className="team-tabs" role="tablist" aria-label="Разделы команды">
          <button role="tab" aria-selected={tab === 'interns'} className={tab === 'interns' ? 'is-selected' : ''} onClick={() => setTab('interns')}>Стажёры</button>
          <button role="tab" aria-selected={tab === 'review'} className={tab === 'review' ? 'is-selected' : ''} onClick={() => setTab('review')}>
            На проверку {pending.length > 0 && <span className="count-badge">{pending.length}</span>}
          </button>
        </div>
      )}

      {isBuddy && tab === 'review' && (
        <section className="staff-card review-queue" aria-label="На проверку">
          {pending.length === 0 && <p className="task-empty">Новых работ нет.</p>}
          {pending.map(item => (
            <button key={item.id} className="queue-row" onClick={() => { setReview(item); setError(''); }}>
              <span><b>{item.item_title}</b><small>{internName(item.intern_id)}</small></span>
              <small>Попытка {item.attempt_number} · {formatDateTime(item.submitted_at)}</small>
            </button>
          ))}
        </section>
      )}

      {(!isBuddy || tab === 'interns') && <div className="team-grid">
        <aside className="staff-card intern-list" aria-label="Стажёры">
          <h2>Стажёры</h2>
          {interns.length === 0 && <p className="task-empty">Нет назначенных стажёров.</p>}
          {interns.map(intern => (
            <button key={intern.id} className={`intern-chip${selected === intern.id ? ' is-selected' : ''}`} aria-pressed={selected === intern.id}
              onClick={() => { setSelected(intern.id); setProgress(undefined); }}>
              <span className="avatar-sm" aria-hidden="true">{personName(intern).charAt(0).toUpperCase()}</span>
              <span><b>{personName(intern)}</b><small>{professionLabel(intern.profession)}</small></span>
            </button>
          ))}
        </aside>

        <section className="staff-card progress-panel" aria-label="Прогресс">
          <header className="panel-head">
            <h2>Прогресс</h2>
            {isLead && selected && progress && (
              <button className="pill pill-black pill-sm" onClick={() => { setTask({ ...EMPTY_TASK, position: totalOf('professional') + 1 }); setTaskOpen(true); }}>+ Задача</button>
            )}
          </header>
          {!selected ? <p className="task-empty">Выберите стажёра.</p> : !progress ? <p className="task-empty">Загружаем…</p> : (
            <>
              <div className="progress-hero">
                <ProgressRing percent={progress.completion_percent} label="Общий прогресс стажёра" size={150} />
                <div><b>{progress.completed_items} из {progress.total_items} задач</b><p>выполнено по двум чек-листам</p></div>
              </div>
              {lists.map(({ title, checklist }) => (
                <div key={title} className="mini-list">
                  <h3>{title}</h3>
                  {checklist.stages.flatMap(stage => stage.items.map(item => ({ item, stage }))).map(({ item, stage }) => (
                    <div key={item.id} className="mini-task-row">
                      <div className={`mini-task is-${columnOf(item)} is-clickable`} role="button" tabIndex={0}
                        onClick={() => setOpenTask(item.id)}
                        onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setOpenTask(item.id); } }}>
                        <span className="mini-title">{item.title}</span>
                        <span className="mini-meta">{item.due_at ? formatDateTime(item.due_at) : ''}</span>
                        <span className={`status-chip is-${columnOf(item)}`}>{statusLabel(item)}</span>
                      </div>
                      {isLead && (
                        <span className="task-tools">
                          <button type="button" aria-label={`Изменить задачу «${item.title}»`}
                            onClick={() => { setError(''); setEdit({ id: item.id, title: item.title, description: item.description, expected_result: item.expected_result ?? '', estimated_minutes: item.estimated_minutes ?? 30, due_at: toLocalInput(item.due_at) }); }}>Изменить</button>
                          <button type="button" className="danger" aria-label={`Удалить задачу «${item.title}»`} onClick={() => { setError(''); setRemoving(item); }}>Удалить</button>
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              ))}
              {isLead && (
                <div className="demo-box">
                  <b>Демо</b>
                  <small>Только для демонстрации: все задачи обоих чек-листов станут выполненными, отправки на проверку принимаются.</small>
                  <button type="button" className="pill pill-dark pill-sm" disabled={demoBusy} onClick={() => void completeDemo()}>
                    {demoBusy ? 'Завершаем…' : 'Завершить чек-листы на 100% (демо)'}
                  </button>
                  {demoNote && <small role="status">{demoNote}</small>}
                </div>
              )}
            </>
          )}
        </section>
      </div>}

      {openTask && selected && (() => {
        const found = lists.flatMap(({ checklist }) => checklist.stages.flatMap(stage => stage.items.map(item => ({ item, stage, checklist }))))
          .find(entry => entry.item.id === openTask);
        return found ? (
          <TaskModal item={found.item} stage={found.stage} checklistName={found.checklist.type === 'common' ? 'Общий' : 'Профессиональный'}
            token={token} internId={selected} onClose={() => setOpenTask('')} />
        ) : null;
      })()}

      {review && (
        <ReviewDialog submission={review} internName={internName(review.intern_id)} token={token} error={error}
          onDecide={(approved, comment) => void decide(approved, comment)} onClose={() => setReview(null)} />
      )}

      {taskOpen && isLead && (
        <Dialog title="Персональная задача" onClose={() => setTaskOpen(false)}>
          <form className="dialog-form" onSubmit={addTask}>
            <label className="field">Название<input required value={task.title} onChange={e => setTask({ ...task, title: e.target.value })} /></label>
            <label className="field">Описание<textarea value={task.description} onChange={e => setTask({ ...task, description: e.target.value })} /></label>
            <label className="field">Ожидаемый результат<textarea required value={task.expected_result} onChange={e => setTask({ ...task, expected_result: e.target.value })} /></label>
            <label className="field">Срок<input type="datetime-local" value={task.due_at} onChange={e => setTask({ ...task, due_at: e.target.value })} /></label>
            <label className="field">Чек-лист
              <select value={task.checklist_type} onChange={e => setTask({ ...task, checklist_type: e.target.value as ChecklistType, position: totalOf(e.target.value as ChecklistType) + 1 })}>
                <option value="professional">Профессиональный</option>
                <option value="common">Общий</option>
              </select>
            </label>
            <label className="field">Позиция в чек-листе
              <select value={task.position} onChange={e => setTask({ ...task, position: Number(e.target.value) })}>
                {Array.from({ length: totalOf(task.checklist_type) + 1 }, (_, i) => (
                  <option key={i} value={i + 1}>{i + 1}{i < totalOf(task.checklist_type) ? ' — перед задачей' : ' — в конец'}</option>
                ))}
              </select>
            </label>
            <label className="field">Оценка, минут<input type="number" min={1} required value={task.estimated_minutes} onChange={e => setTask({ ...task, estimated_minutes: Number(e.target.value) })} /></label>
            <label className="check"><input type="checkbox" checked={task.requires_review} onChange={e => setTask({ ...task, requires_review: e.target.checked })} />Требуется проверка наставника</label>
            {error && <p className="state-message state-error" role="alert">{error}</p>}
            <div className="dialog-actions"><button className="pill pill-black pill-sm">Создать задачу</button></div>
          </form>
        </Dialog>
      )}

      {edit && isLead && (
        <Dialog title="Изменить задачу" onClose={() => setEdit(null)}>
          <form className="dialog-form" onSubmit={saveEdit}>
            <label className="field">Название<input required value={edit.title} onChange={e => setEdit({ ...edit, title: e.target.value })} /></label>
            <label className="field">Описание<textarea value={edit.description} onChange={e => setEdit({ ...edit, description: e.target.value })} /></label>
            <label className="field">Ожидаемый результат<textarea value={edit.expected_result} onChange={e => setEdit({ ...edit, expected_result: e.target.value })} /></label>
            <label className="field">Срок<input type="datetime-local" value={edit.due_at} onChange={e => setEdit({ ...edit, due_at: e.target.value })} /></label>
            <label className="field">Оценка, минут<input type="number" min={1} required value={edit.estimated_minutes} onChange={e => setEdit({ ...edit, estimated_minutes: Number(e.target.value) })} /></label>
            {error && <p className="state-message state-error" role="alert">{error}</p>}
            <div className="dialog-actions"><button className="pill pill-black pill-sm">Сохранить</button></div>
          </form>
        </Dialog>
      )}

      {removing && isLead && (
        <Dialog title="Удалить задачу" onClose={() => setRemoving(null)}>
          <p>Убрать «{removing.title}» из чек-листа этого стажёра? Отправки и комментарии сохранятся.</p>
          <div className="dialog-actions">
            <button type="button" className="pill pill-dark pill-sm" onClick={() => setRemoving(null)}>Отмена</button>
            <button type="button" className="pill pill-black pill-sm" onClick={() => void confirmRemove()}>Удалить</button>
          </div>
        </Dialog>
      )}
    </div>
  );
}
