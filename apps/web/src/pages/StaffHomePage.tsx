import { useCallback, useEffect, useState } from 'react';
import { getAssignedInterns, getInternProgress, getPendingSubmissions, type ProgressResponse, type User } from '@ai-buddy/client';
import { bot } from '../bot/api';
import { ProgressRing } from '../components/home/ProgressRing';
import type { View } from '../components/shell/Sidebar';
import { displayName, isDone, personName, professionLabel } from '../lib/format';

type Props = { user: User; token: string; onNavigate: (view: View) => void };
type Stats = { review: number; questions: number; requests: number };

/** Главная наставника и руководителя: что требует внимания и как идут стажёры. */
export function StaffHomePage({ user, token, onNavigate }: Props) {
  const [interns, setInterns] = useState<User[]>([]);
  const [progress, setProgress] = useState<Record<string, ProgressResponse>>({});
  const [stats, setStats] = useState<Stats>({ review: 0, questions: 0, requests: 0 });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const list = await getAssignedInterns('', token);
      setInterns(list);
      const entries = await Promise.all(list.map(async intern => [intern.id, await getInternProgress('', token, intern.id)] as const));
      setProgress(Object.fromEntries(entries));
      const [review, questions, requests] = await Promise.all([
        getPendingSubmissions('', token).catch(() => []),
        bot.questions(token, 'open').catch(() => []),
        bot.staffRequests(token).catch(() => []),
      ]);
      setStats({ review: review.length, questions: questions.length, requests: requests.filter(r => r.request.status === 'pending_approval').length });
      setError('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось загрузить данные');
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 30000);
    return () => clearInterval(timer);
  }, [load]);

  const greeting = user.first_name || displayName(user.email);
  const cards: { label: string; value: number; to: View; hint: string }[] = [
    { label: 'Стажёров', value: interns.length, to: 'team', hint: 'в вашей команде' },
    ...(user.role === 'buddy' ? [{ label: 'Ждут проверки', value: stats.review, to: 'team' as View, hint: 'задания с результатом' }] : []),
    { label: 'Вопросов от бота', value: stats.questions, to: 'mentor', hint: 'ждут вашего ответа' },
    { label: 'Заявок', value: stats.requests, to: 'mentor', hint: 'на согласование' },
  ];

  return (
    <div className="home staff-home">
      <header className="greeting">
        <h1>Привет, {greeting}!</h1>
        <p>{user.role === 'buddy' ? 'Что бот и стажёры передали вам сегодня' : 'Как идёт адаптация вашей команды'}</p>
      </header>

      {error && <p className="state-message state-error" role="alert">{error} <button className="link" onClick={() => void load()}>Повторить</button></p>}

      <div className="stat-row">
        {cards.map((card, index) => (
          <button key={card.label} className={`stat-card${card.value > 0 && index > 0 ? ' is-attention' : ''}`} onClick={() => onNavigate(card.to)}>
            <span className="stat-label">{card.label}</span>
            <strong className="stat-value">{card.value}</strong>
            <span className="stat-hint">{card.hint}</span>
          </button>
        ))}
      </div>

      <section className="task-table staff-table" aria-label="Стажёры">
        <header><h2>Стажёры</h2><button className="link" onClick={() => onNavigate('team')}>Все подробности</button></header>
        <div className="intern-columns" aria-hidden="true"><span>Стажёр</span><span>Направление</span><span>Прогресс</span><span>Следующая задача</span></div>
        {loading && interns.length === 0 && <p className="task-empty">Загружаем…</p>}
        {!loading && interns.length === 0 && <p className="task-empty">Пока нет назначенных стажёров.</p>}
        {interns.map(intern => {
          const data = progress[intern.id];
          const next = data?.professional.stages.flatMap(stage => stage.items).find(item => !isDone(item));
          return (
            <article key={intern.id} className="intern-row" onClick={() => onNavigate('team')}>
              <span className="intern-name"><span className="avatar-sm" aria-hidden="true">{personName(intern).charAt(0).toUpperCase()}</span>{personName(intern)}</span>
              <span className="intern-cell">{professionLabel(intern.profession)}</span>
              <span className="intern-cell intern-progress">
                <ProgressRing percent={data?.completion_percent ?? 0} label={`Прогресс: ${personName(intern)}`} size={56} />
                <small>{data ? `${data.completed_items}/${data.total_items}` : '…'}</small>
              </span>
              <span className="intern-cell">{next ? next.title : data ? 'Все задачи выполнены' : '…'}</span>
            </article>
          );
        })}
      </section>
    </div>
  );
}
