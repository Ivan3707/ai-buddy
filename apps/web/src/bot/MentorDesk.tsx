import { useCallback, useEffect, useState } from 'react';
import { bot, REQUEST_STATUS, type ChatMetrics, type GapsReport, type KbLintReport, type MentorDraft, type MentorQuestion, type ServiceRequest } from './api';

// Buddy / teamlead side of the bot: what the assistant escalated, AI drafts, request approvals and the effect metrics.

const err = (e: unknown) => e instanceof Error ? e.message : 'Ошибка';
const when = (iso: string) => new Date(iso).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
const PARAMS: Record<string, string> = { day_off: 'отгул', vacation: 'отпуск', read: 'чтение', write: 'запись', admin: 'админ' };
const describe = (r: ServiceRequest) => r.type === 'leave_request'
  ? `${PARAMS[r.params.kind] ?? r.params.kind}: ${r.params.date_from}${r.params.date_to !== r.params.date_from ? ` — ${r.params.date_to}` : ''}`
  : `${r.params.system} · ${r.params.resource} · ${PARAMS[r.params.access_level] ?? r.params.access_level}`;

function Question({ q, token, reload }: { q: MentorQuestion; token: string; reload: () => void }) {
  const [draft, setDraft] = useState<MentorDraft | null>(null); const [text, setText] = useState('');
  const [busy, setBusy] = useState(''); const [error, setError] = useState('');
  const makeDraft = async () => { setBusy('draft'); setError(''); try { const d = await bot.draft(token, q.id); setDraft(d); setText(d.draft); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  const send = async () => { setBusy('send'); setError(''); try { await bot.answer(token, q.id, text.trim()); reload(); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  return <article className="desk-item">
    <header><b>{q.intern_name || 'Стажёр'}</b><span className={`pill ${q.status === 'open' ? 'pending' : 'completed'}`}>{q.status === 'open' ? 'ждёт ответа' : 'отвечен'}</span><small className="muted">{when(q.created_at)}</small></header>
    <p className="desk-question">{q.question}</p>
    {q.context && <p className="muted desk-context">Контекст от бота: {q.context}</p>}
    {q.answer && <p className="notice success">Ваш ответ: {q.answer}</p>}
    {q.status === 'open' && !draft && <button className="ghost" disabled={busy === 'draft'} onClick={() => void makeDraft()}>{busy === 'draft' ? 'Готовлю черновик…' : '✦ AI-черновик ответа'}</button>}
    {q.status === 'open' && draft && <div className="desk-draft">
      <p className="muted">Уверенность: <b>{({ grounded: 'есть в базе знаний', partial: 'частично', none: 'в базе нет' } as const)[draft.confidence]}</b>
        {!!draft.sources.length && <> · источники: {draft.sources.map(s => s.title).join('; ')}</>}</p>
      <textarea value={text} onChange={e => setText(e.target.value)}/>
      <button disabled={busy === 'send' || !text.trim()} onClick={() => void send()}>{busy === 'send' ? 'Отправляем…' : 'Отправить стажёру'}</button>
      {draft.kb_suggestion && <details className="desk-kb"><summary>Предложение статьи в базу знаний: {draft.kb_suggestion.title}</summary><pre>{draft.kb_suggestion.text}</pre></details>}
    </div>}
    {error && <p className="notice error">{error}</p>}
  </article>;
}

export function MentorDesk({ token }: { token: string }) {
  const [metrics, setMetrics] = useState<ChatMetrics | null>(null);
  const [questions, setQuestions] = useState<MentorQuestion[]>([]);
  const [requests, setRequests] = useState<{ request: ServiceRequest; intern_name: string }[]>([]);
  const [tab, setTab] = useState<'open' | 'answered'>('open');
  const [gaps, setGaps] = useState<GapsReport | null>(null); const [lint, setLint] = useState<KbLintReport | null>(null);
  const [busy, setBusy] = useState(''); const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [m, q, r] = await Promise.all([bot.metrics(token), bot.questions(token), bot.staffRequests(token)]);
      setMetrics(m); setQuestions(q); setRequests(r); setError('');
    } catch (e) { setError(err(e)); }
  }, [token]);
  useEffect(() => { void load(); const id = setInterval(() => void load(), 15000); return () => clearInterval(id); }, [load]);

  const decide = async (id: string, status: 'approved' | 'rejected') => { setBusy(id); try { await bot.decide(token, id, status); await load(); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  const runGaps = async () => { setBusy('gaps'); try { setGaps(await bot.gaps(token)); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  const runLint = async () => { setBusy('lint'); try { setLint(await bot.lint(token)); } catch (e) { setError(err(e)); } finally { setBusy(''); } };
  const shown = questions.filter(q => q.status === tab);
  const open = questions.filter(q => q.status === 'open').length;
  const pending = requests.filter(r => r.request.status === 'pending_approval');

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

    <section className="panel">
      <header className="desk-head"><div><span className="eyebrow">Эскалации от бота</span><h2>Вопросы стажёров {open > 0 && <span className="count">{open}</span>}</h2></div>
        <div className="desk-tabs"><button className={tab === 'open' ? '' : 'ghost'} onClick={() => setTab('open')}>Ждут ответа</button><button className={tab === 'answered' ? '' : 'ghost'} onClick={() => setTab('answered')}>Отвеченные</button></div></header>
      {shown.length ? shown.map(q => <Question key={q.id} q={q} token={token} reload={() => void load()}/>) : <p className="muted">{tab === 'open' ? 'Новых вопросов нет — бот справляется сам.' : 'Пока нет отвеченных.'}</p>}
    </section>

    <section className="panel">
      <span className="eyebrow">Корпоративные сервисы (мок)</span><h2>Заявки стажёров {pending.length > 0 && <span className="count">{pending.length}</span>}</h2>
      {requests.length ? requests.map(({ request: r, intern_name }) => <div className="desk-request" key={r.id}>
        <div><b>{r.number}</b> · {intern_name || 'Стажёр'}<br/><small className="muted">{describe(r)}{r.params.justification ? ` · ${r.params.justification}` : ''}</small></div>
        {r.status === 'pending_approval'
          ? <div className="desk-tabs"><button disabled={busy === r.id} onClick={() => void decide(r.id, 'approved')}>Согласовать</button><button className="ghost" disabled={busy === r.id} onClick={() => void decide(r.id, 'rejected')}>Отклонить</button></div>
          : <span className={`pill ${r.status === 'approved' ? 'completed' : ''}`}>{REQUEST_STATUS[r.status]}</span>}
      </div>) : <p className="muted">Заявок пока нет. Стажёр оформляет их в чате с ботом.</p>}
    </section>

    <section className="panel">
      <span className="eyebrow">Качество базы знаний</span><h2>Где бот не справляется</h2>
      <div className="desk-tabs"><button className="ghost" disabled={busy === 'gaps'} onClick={() => void runGaps()}>{busy === 'gaps' ? 'Считаю…' : 'Пробелы: темы вопросов без ответа'}</button>
        <button className="ghost" disabled={busy === 'lint'} onClick={() => void runLint()}>{busy === 'lint' ? 'Проверяю (до минуты)…' : 'Линтер: противоречия и устаревшее'}</button></div>
      {gaps && (gaps.clusters.length ? <ul>{gaps.clusters.map(c => <li key={c.label}><b>{c.label}</b> — {c.count}: <span className="muted">{c.examples.join(' / ')}</span></li>)}</ul> : <p className="muted">Эскалированных вопросов пока нет.</p>)}
      {lint && (lint.issues.length ? <ul>{lint.issues.map((i, n) => <li key={n}><span className="pill">{i.kind === 'contradiction' ? 'противоречие' : i.kind === 'stale' ? 'устарело' : i.kind}</span> <b>{i.doc_ids.join(' ↔ ')}</b>: {i.detail}</li>)}</ul> : <p className="muted">Проблем не найдено.</p>)}
    </section>
  </div>;
}
