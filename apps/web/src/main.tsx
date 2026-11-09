import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { chat, type ChatResponse } from '@ai-buddy/client';
import './style.css';

type Scenario = {id:string; title:string; tasks:{id:string; title:string}[]};
function App() {
  const [role, setRole] = useState('ml');
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [done, setDone] = useState<Record<string, boolean>>({});
  const [message, setMessage] = useState('Как оформить отпуск?');
  const [reply, setReply] = useState<ChatResponse>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    fetch(`/api/onboarding?role=${role}`, {signal:controller.signal})
      .then(r => {if (!r.ok) throw new Error('Не удалось загрузить сценарии'); return r.json();})
      .then(setScenarios).catch(e => {if (e.name !== 'AbortError') setError(e.message);});
    return () => controller.abort();
  }, [role]);
  return <main><header><span>Hack&Change 2026 · DEMO</span><h1>AI-buddy</h1><p>Первые шаги в команде, по одной задаче за раз.</p></header>
    <section><h2>Мой онбординг</h2><label>Роль <select value={role} onChange={e => setRole(e.target.value)}>
      <option value="ml">ML-инженер</option><option value="backend">Backend-разработчик</option><option value="frontend">Frontend-разработчик</option>
    </select></label><p>Прогресс хранится только до обновления страницы.</p>
    {scenarios.map(s => <article key={s.id}><h3>{s.title}</h3>{s.tasks.map(t => <label className="task" key={t.id}><input type="checkbox" checked={!!done[`${s.id}:${t.id}`]} onChange={e => setDone({...done, [`${s.id}:${t.id}`]:e.target.checked})}/>{t.title}</label>)}</article>)}</section>
    <section><h2>Спросить помощника</h2><form onSubmit={async e => {e.preventDefault();setBusy(true);setError('');setReply(undefined);try {setReply(await chat('', message));} catch {setError('Backend недоступен. Проверьте запуск сервисов.');} finally {setBusy(false);}}}>
      <label htmlFor="message">Ваш вопрос</label><textarea id="message" maxLength={2000} required value={message} onChange={e => setMessage(e.target.value)}/>
      <button disabled={busy || !message.trim()}>{busy ? 'Ищем…' : 'Отправить'}</button></form>
      {reply && <div aria-live="polite"><p>{reply.answer}</p>{reply.sources.map(s => <a key={s.id} href={s.url} target="_blank" rel="noreferrer">{s.title} (демо-ссылка)</a>)}</div>}
      {error && <p role="alert">{error}</p>}</section><footer>Синтетические данные · Mock AI · Корпоративные сервисы не подключены</footer></main>;
}
createRoot(document.getElementById('root')!).render(<React.StrictMode><App/></React.StrictMode>);
