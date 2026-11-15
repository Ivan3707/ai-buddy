import { useCallback, useEffect, useState } from 'react';
import { bot, type ChatMetrics, type MentorDraft, type MentorQuestion } from './api';

// Mentor (buddy) side of the bot: what the assistant escalated, AI drafts and the effect metrics. Requests are approved by the teamlead (RequestsDesk), knowledge-base quality is in the admin panel.

const err = (e: unknown) => e instanceof Error ? e.message : 'Ошибка';
const when = (iso: string) => new Date(iso).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
function Question({ q, token, reload }: { q: MentorQuestion; token: string; reload: () => void }) {
  const [draft, setDraft] = useState<MentorDraft | null>(null); const [text, setText] = useState('');
  const [manual, setManual] = useState(false); const [busy, setBusy] = useState(''); const [error, setError] = useState('');
  const makeDraft = async () => { setBusy('draft'); setError(''); try { const d = await bot.draft(token, q.id); setDraft(d); setText(d.draft); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  const send = async () => { setBusy('send'); setError(''); try { await bot.answer(token, q.id, text.trim()); reload(); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  return <article className="desk-item">
    <header><b>{q.intern_name || 'Стажёр'}</b><span className={`chip ${q.status === 'open' ? 'pending' : 'completed'}`}>{q.status === 'open' ? 'ждёт ответа' : 'отвечен'}</span><small className="muted">{when(q.created_at)}</small></header>
    <p className="desk-question">{q.question}</p>
    {q.context && <p className="muted desk-context">Контекст от бота: {q.context}</p>}
    {q.answer && <p className="notice success">Ваш ответ: {q.answer}</p>}
    {q.status === 'open' && !draft && !manual && <div className="desk-actions">
      <button className="btn-ghost" disabled={busy === 'draft'} onClick={() => void makeDraft()}>{busy === 'draft' ? 'Готовлю черновик…' : '✦ AI-черновик ответа'}</button>
      <button onClick={() => setManual(true)}>Ответить</button>
    </div>}
    {q.status === 'open' && (draft || manual) && <div className="desk-draft">
      {draft && <p className="muted">Уверенность: <b>{({ grounded: 'есть в базе знаний', partial: 'частично', none: 'в базе нет' } as const)[draft.confidence]}</b>
        {!!draft.sources.length && <> · источники: {draft.sources.map(s => s.title).join('; ')}</>}</p>}
      <textarea aria-label="Ваш ответ" placeholder="Напишите ответ стажёру…" value={text} onChange={e => setText(e.target.value)}/>
      <button disabled={busy === 'send' || !text.trim()} onClick={() => void send()}>{busy === 'send' ? 'Отправляем…' : 'Отправить стажёру'}</button>
      {draft?.kb_suggestion && <details className="desk-kb"><summary>Предложение статьи в базу знаний: {draft.kb_suggestion.title}</summary><pre>{draft.kb_suggestion.text}</pre></details>}
    </div>}
    {error && <p className="notice error">{error}</p>}
  </article>;
}

export function MentorDesk({ token }: { token: string }) {
  const [metrics, setMetrics] = useState<ChatMetrics | null>(null);
  const [questions, setQuestions] = useState<MentorQuestion[]>([]);
  const [tab, setTab] = useState<'open' | 'answered'>('open');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [m, q] = await Promise.all([bot.metrics(token), bot.questions(token)]);
      setMetrics(m); setQuestions(q); setError('');
    } catch (e) { setError(err(e)); }
  }, [token]);
  useEffect(() => { void load(); const id = setInterval(() => void load(), 15000); return () => clearInterval(id); }, [load]);

  const shown = questions.filter(q => q.status === tab);
  const open = questions.filter(q => q.status === 'open').length;

  return <div className="desk">
    {error && <p className="notice error">{error}</p>}
    {metrics && <section className="desk-metrics">
      <div><b>{metrics.questions_total}</b><span>вопросов боту</span></div>
      <div><b>{metrics.answered_by_bot}</b><span>закрыл сам</span></div>
      <div><b>{metrics.escalated_to_mentor}</b><span>передал вам</span></div>
      <div><b>{metrics.deflection_rate === null ? '—' : `${Math.round(metrics.deflection_rate * 100)}%`}</b><span>без наставника</span></div>
      <div><b>{metrics.actions_executed}</b><span>действий выполнено</span></div>
      <div className="accent"><b>≈ {metrics.estimated_mentor_minutes_saved} мин</b><span>вашего времени сэкономлено</span></div>
    </section>}

    <section className="staff-card">
      <header className="desk-head"><div><span className="eyebrow">Эскалации от бота</span><h2>Вопросы стажёров {open > 0 && <span className="count-badge">{open}</span>}</h2></div>
        <div className="desk-tabs"><button className={tab === 'open' ? '' : 'btn-ghost'} onClick={() => setTab('open')}>Ждут ответа</button><button className={tab === 'answered' ? '' : 'btn-ghost'} onClick={() => setTab('answered')}>Отвеченные</button></div></header>
      {shown.length ? shown.map(q => <Question key={q.id} q={q} token={token} reload={() => void load()}/>) : <p className="muted">{tab === 'open' ? 'Новых вопросов нет — бот справляется сам.' : 'Пока нет отвеченных.'}</p>}
    </section>

  </div>;
}
