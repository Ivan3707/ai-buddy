import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  createPersonalTask, getAssignedInterns, getInternProgress, getPendingSubmissions, reviewSubmission,
  type ChecklistStatus, type PendingSubmission, type ProgressResponse, type User,
} from '@ai-buddy/client';
import { ProgressRing } from '../components/home/ProgressRing';
import { Dialog } from '../components/staff/Dialog';
import { formatDateTime, personName, professionLabel } from '../lib/format';

type Props = { user: User; token: string };

const STATUS: Record<ChecklistStatus, string> = { unavailable: 'Недоступно', active: 'В работе', completed: 'Выполнено', verified: 'Проверено' };
const EMPTY_TASK = { title: '', description: '', expected_result: '', due_at: '', position: 1, estimated_minutes: 30, requires_review: false };
const message = (reason: unknown) => (reason instanceof Error ? reason.message : 'Ошибка');

/** Команда: стажёры, их прогресс, очередь проверок наставника и добавление задач руководителем. */
export function StaffTeamPage({ user, token }: Props) {
  const [interns, setInterns] = useState<User[]>([]);
  const [selected, setSelected] = useState('');
  const [progress, setProgress] = useState<ProgressResponse>();
  const [error, setError] = useState('');
  const [taskOpen, setTaskOpen] = useState(false);
  const [task, setTask] = useState(EMPTY_TASK);
  const [pending, setPending] = useState<PendingSubmission[]>([]);
  const [review, setReview] = useState<PendingSubmission | null>(null);
  const [comment, setComment] = useState('');
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

  const professionalTotal = progress?.professional.total_items ?? 0;
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
  const decide = async (approved: boolean) => {
    if (!review) return;
    if (!approved && !comment.trim()) { setError('Для возврата нужен комментарий'); return; }
    try {
      await reviewSubmission('', token, review.id, { approved, comment: approved ? undefined : comment.trim() });
      setReview(null); setComment(''); setError('');
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

      <div className="team-grid">
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
              <button className="pill pill-black pill-sm" onClick={() => { setTask({ ...EMPTY_TASK, position: professionalTotal + 1 }); setTaskOpen(true); }}>+ Задача</button>
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
                  {checklist.stages.flatMap(stage => stage.items).map(item => (
                    <div key={item.id} className={`mini-task is-${item.status}`}>
                      <span className="mini-title">{item.title}</span>
                      <span className="mini-meta">{item.due_at ? formatDateTime(item.due_at) : ''}</span>
                      <span className={`status-chip is-${item.status}`}>{STATUS[item.status]}</span>
                    </div>
                  ))}
                </div>
              ))}
            </>
          )}
        </section>
      </div>

      {isBuddy && (
        <section className="staff-card review-queue" aria-label="Очередь проверок">
          <header className="panel-head"><h2>Очередь проверок</h2><span className="count-badge">{pending.length}</span></header>
          {pending.length === 0 && <p className="task-empty">Новых работ нет.</p>}
          {pending.map(item => (
            <button key={item.id} className="queue-row" onClick={() => { setReview(item); setComment(''); setError(''); }}>
              <span>{item.item_title}</span><small>Попытка {item.attempt_number} · {formatDateTime(item.submitted_at)}</small>
            </button>
          ))}
        </section>
      )}

      {review && (
        <Dialog title={review.item_title} onClose={() => setReview(null)}>
          <p><b>Попытка:</b> {review.attempt_number}</p>
          <p className="review-result"><b>Результат:</b> {review.result_text || (review.result_url ? <a href={review.result_url} target="_blank" rel="noreferrer">{review.result_url}</a> : 'Не указан')}</p>
          <label className="field">Комментарий при возврате<textarea value={comment} onChange={event => setComment(event.target.value)} /></label>
          {error && <p className="state-message state-error" role="alert">{error}</p>}
          <div className="dialog-actions">
            <button className="pill pill-dark pill-sm" onClick={() => void decide(false)}>Вернуть</button>
            <button className="pill pill-black pill-sm" onClick={() => void decide(true)}>Принять</button>
          </div>
        </Dialog>
      )}

      {taskOpen && isLead && (
        <Dialog title="Персональная задача" onClose={() => setTaskOpen(false)}>
          <form className="dialog-form" onSubmit={addTask}>
            <label className="field">Название<input required value={task.title} onChange={e => setTask({ ...task, title: e.target.value })} /></label>
            <label className="field">Описание<textarea value={task.description} onChange={e => setTask({ ...task, description: e.target.value })} /></label>
            <label className="field">Ожидаемый результат<textarea required value={task.expected_result} onChange={e => setTask({ ...task, expected_result: e.target.value })} /></label>
            <label className="field">Срок<input type="datetime-local" value={task.due_at} onChange={e => setTask({ ...task, due_at: e.target.value })} /></label>
            <label className="field">Позиция в профессиональном чек-листе
              <select value={task.position} onChange={e => setTask({ ...task, position: Number(e.target.value) })}>
                {Array.from({ length: professionalTotal + 1 }, (_, i) => (
                  <option key={i} value={i + 1}>{i + 1}{i < professionalTotal ? ' — перед задачей' : ' — в конец'}</option>
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
    </div>
  );
}
