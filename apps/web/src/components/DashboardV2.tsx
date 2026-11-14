import { useEffect, useState } from 'react';
import { getAssignedInterns, getChecklists, getInternProgress, getPendingSubmissions, getProgress, listEvents, type CalendarEvent, type ChecklistItem, type PendingSubmission, type ProgressResponse, type User } from '@ai-buddy/client';
import { Page } from './Pages';

const date = (value: string) => new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
export function DashboardPage({ token, user }: { token: string; user: User }) {
  const [progress, setProgress] = useState<ProgressResponse>();
  const [interns, setInterns] = useState<User[]>([]);
  const [teamProgress, setTeamProgress] = useState<Record<string, ProgressResponse>>({});
  const [tasks, setTasks] = useState<ChecklistItem[]>([]);
  const [pending, setPending] = useState<PendingSubmission[]>([]);
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => { let active = true; (async () => {
    try {
      if (user.role === 'intern') {
        const [p, checklists, calendar] = await Promise.all([getProgress('', token), getChecklists('', token), listEvents('', token)]);
        if (active) { setProgress(p); setTasks([...checklists.common.stages.flatMap(stage => stage.items), ...checklists.professional.stages.flatMap(stage => stage.items)]); setEvents(calendar); }
      } else {
        const people = await getAssignedInterns('', token);
        const [summaries, queue, calendar] = await Promise.all([Promise.all(people.map(person => getInternProgress('', token, person.id).then(value => [person.id, value] as const))), user.role === 'buddy' ? getPendingSubmissions('', token) : Promise.resolve([]), listEvents('', token)]);
        if (active) { setInterns(people); setTeamProgress(Object.fromEntries(summaries)); setPending(queue); setEvents(calendar); }
      }
    } catch (reason) { if (active) setError(reason instanceof Error ? reason.message : 'Не удалось загрузить данные'); }
    finally { if (active) setLoading(false); }
  })(); return () => { active = false; }; }, [token, user.role]);
  const upcoming = events.filter(event => new Date(event.ends_at) >= new Date()).sort((a,b) => a.starts_at.localeCompare(b.starts_at)).slice(0, 4);
  const deadlines = tasks.filter(task => task.due_at && task.status !== 'completed' && task.status !== 'verified').sort((a,b) => (a.due_at || '').localeCompare(b.due_at || '')).slice(0, 4);
  const activeTasks = tasks.filter(task => task.status === 'active').slice(0, 4);
  const teamAttention = interns.flatMap(person => (teamProgress[person.id]?.professional.stages ?? []).flatMap(stage => stage.items.filter(item => item.status === 'active' || (item.due_at && item.status !== 'completed' && item.status !== 'verified')).map(item => ({ person, item })))).sort((a,b) => (a.item.due_at || '9999').localeCompare(b.item.due_at || '9999')).slice(0, 5);
  return <Page title={user.first_name ? `Привет, ${user.first_name}` : 'Главная'} eyebrow="Мой день" subtitle="Данные из доступных вам ответов Backend.">{loading && <p>Загружаем…</p>}{error && <p className="notice error" role="alert">{error}</p>}{!loading && !error && <div className="dashboard-panels">
    {user.role === 'intern' ? <><section className="panel"><h2>Прогресс</h2><strong className="big-number">{Math.round(progress?.completion_percent ?? 0)}%</strong><p>{progress?.completed_items} из {progress?.total_items} задач</p><a href="/checklists">Открыть чек-листы →</a></section><section className="panel"><h2>Доступные задачи</h2>{activeTasks.length ? activeTasks.map(task => <p key={task.id}>{task.title}</p>) : <p className="muted">Нет доступных задач.</p>}</section><section className="panel"><h2>Ближайшие сроки</h2>{deadlines.length ? deadlines.map(task => <p key={task.id}>{task.title} · {date(task.due_at!)}</p>) : <p className="muted">Сроки не заданы.</p>}</section></> : <><section className="panel"><h2>Стажёры {interns.length}</h2>{interns.length ? interns.map(person => <p key={person.id}>{person.first_name || person.email} · {Math.round(teamProgress[person.id]?.completion_percent ?? 0)}%</p>) : <p className="muted">Назначенных стажёров нет.</p>}<a href="/team">Перейти к команде →</a></section>{user.role === 'buddy' ? <section className="panel"><h2>Ожидают проверки {pending.length}</h2>{pending.slice(0,4).map(item => <p key={item.id}>{item.item_title}</p>)}<a href="/team">Открыть очередь →</a></section> : <section className="panel"><h2>Профессиональные треки</h2>{interns.map(person => <p key={person.id}>{person.first_name || person.email} · {teamProgress[person.id]?.professional.completed_items ?? 0} из {teamProgress[person.id]?.professional.total_items ?? 0}</p>)}<a href="/team">Открыть команду →</a></section>}</>}
    {user.role === 'teamlead' && <section className="panel"><h2>Требуют внимания</h2>{teamAttention.length ? teamAttention.map(({person,item}) => <p key={`${person.id}-${item.id}`}>{person.first_name || person.email}: {item.title}{item.due_at ? ` · до ${date(item.due_at)}` : ' · доступно'}</p>) : <p className="muted">Активных задач и сроков нет.</p>}<a href="/team">Открыть команду →</a></section>}
    <section className="panel"><h2>Ближайшие события</h2>{upcoming.length ? upcoming.map(event => <p key={event.id}>{event.title} · {date(event.starts_at)}</p>) : <p className="muted">Предстоящих событий нет.</p>}<a href="/calendar">Открыть календарь →</a></section>
  </div>}</Page>;
}
