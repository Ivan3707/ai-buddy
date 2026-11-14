import { useCallback, useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import type { User } from '@ai-buddy/client';
import { bot, REQUEST_STATUS, type ActionResult, type PendingAction, type Source, type StreamEvent } from './api';
import { Markdown } from './Markdown';

type Msg = {
  key: string; id?: string; role: 'user' | 'assistant'; content: string; sources?: Source[];
  action?: PendingAction; result?: ActionResult; actionError?: string; feedback?: 'up' | 'down' | null;
  stage?: string; streaming?: boolean; error?: string; brief?: boolean;
};

const STAGE: Record<string, string> = { retrieving: 'Ищу в базе знаний…', generating: 'Пишу ответ…', preparing_action: 'Готовлю заявку…' };
const SUGGESTIONS_INTERN = ['Что мне сделать сегодня?', 'Как оформить отгул?', 'Как получить доступ к Grafana?', 'Проверь меня по теме доступов'];
const SUGGESTIONS_STAFF = ['Как оформить отпуск?', 'Что делать, если ревью затянулось?', 'Какие правила работы с данными?'];
const LABELS: Record<string, string> = {
  kind: 'Вид', date_from: 'С', date_to: 'По', comment: 'Комментарий', system: 'Система', resource: 'Ресурс',
  access_level: 'Уровень', justification: 'Зачем', question: 'Вопрос', context: 'Контекст',
};
const VALUES: Record<string, string> = { day_off: 'Отгул', vacation: 'Отпуск', read: 'чтение', write: 'запись', admin: 'админ' };
const TITLES: Record<string, string> = { leave_request: 'Заявка на отсутствие', access_request: 'Заявка на доступ', mentor_question: 'Вопрос наставнику' };

const today = () => new Date().toISOString().slice(0, 10);
const fmt = (key: string, value: string) => /^\d{4}-\d{2}-\d{2}$/.test(value) ? value.split('-').reverse().join('.') : (key === 'kind' || key === 'access_level' ? VALUES[value] ?? value : value);
let seq = 0; const nextKey = () => `m${Date.now()}-${seq++}`;
function storage<T>(fn: () => T, fallback: T): T { try { return fn(); } catch { return fallback; } }

function ActionCard({ msg, token, onChange }: { msg: Msg; token: string; onChange: (patch: Partial<Msg>) => void }) {
  const [busy, setBusy] = useState(false);
  const action = msg.action!;
  const run = async (kind: 'confirm' | 'cancel') => {
    setBusy(true);
    try {
      if (kind === 'confirm') { const result = await bot.confirm(token, action.id); onChange({ action: result.action, result, actionError: undefined }); }
      else onChange({ action: await bot.cancel(token, action.id) });
    } catch (e) {
      onChange({ actionError: e instanceof Error ? e.message : 'Не удалось выполнить', action: kind === 'confirm' ? { ...action, status: 'failed' } : action });
    } finally { setBusy(false); }
  };
  const request = msg.result?.request; const question = msg.result?.mentor_question;
  return <div className={`bot-card ${action.status}`}>
    <b>{TITLES[action.type]}</b>
    <p className="bot-card-summary">{action.summary}</p>
    <dl>{Object.keys(LABELS).filter(k => action.params[k]).map(k => [k, action.params[k]] as const).map(([k, v]) => <div key={k}><dt>{LABELS[k] ?? k}</dt><dd>{fmt(k, String(v))}</dd></div>)}</dl>
    {action.status === 'pending' && <div className="bot-card-buttons">
      <button disabled={busy} onClick={() => void run('confirm')}>{busy ? '…' : action.type === 'mentor_question' ? 'Отправить наставнику' : 'Подтвердить'}</button>
      <button className="ghost" disabled={busy} onClick={() => void run('cancel')}>Отменить</button>
    </div>}
    {action.status === 'executed' && request && <p className="notice success">Заявка <b>{request.number}</b> создана · {REQUEST_STATUS[request.status]}{request.approver ? ` · согласует: ${request.approver}` : ''}</p>}
    {action.status === 'executed' && question && <p className="notice success">Вопрос отправлен наставнику. Ответ придёт сюда и в уведомления.</p>}
    {action.status === 'executed' && !request && !question && <p className="notice success">Выполнено</p>}
    {action.status === 'cancelled' && <p className="notice">Отменено — ничего не отправлено.</p>}
    {(action.status === 'failed' || msg.actionError) && <p className="notice error">{msg.actionError || action.error || 'Не выполнено'}</p>}
  </div>;
}

export function BotChat({ token, user, compact = false, initialQuestion, onQuestionUsed }: {
  token: string; user: User; compact?: boolean; initialQuestion?: string; onQuestionUsed?: () => void;
}) {
  const convKey = `ai-buddy-conversation:${user.id}`; const briefKey = `ai-buddy-brief:${user.id}`;
  const [conversationId, setConversationId] = useState<string | undefined>(() => storage(() => sessionStorage.getItem(convKey) || undefined, undefined));
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const abort = useRef<AbortController | null>(null);
  const bottom = useRef<HTMLDivElement>(null);
  const queue = useRef<string[]>([]);
  const intern = user.role === 'intern';

  const patch = useCallback((key: string, change: Partial<Msg> | ((m: Msg) => Partial<Msg>)) =>
    setMessages(list => list.map(m => m.key === key ? { ...m, ...(typeof change === 'function' ? change(m) : change) } : m)), []);
  const remember = useCallback((id: string) => { setConversationId(id); storage(() => sessionStorage.setItem(convKey, id), undefined); }, [convKey]);

  const consume = useCallback((key: string) => (e: StreamEvent) => {
    switch (e.event) {
      case 'meta': remember(e.data.conversation_id); patch(key, { id: e.data.message_id }); break;
      case 'status': patch(key, { stage: STAGE[e.data.stage] }); break;
      case 'delta': patch(key, m => ({ content: m.content + e.data.text, stage: undefined })); break;
      case 'sources': patch(key, { sources: e.data.sources }); break;
      case 'action': patch(key, { action: e.data.action }); break;
      case 'error': patch(key, { error: e.data.message }); break;
      case 'done': break;
    }
  }, [patch, remember]);

  const tokenRef = useRef(token); tokenRef.current = token;
  const sendRef = useRef<(text: string, shown?: boolean) => Promise<void>>(async () => undefined);

  const runStream = useCallback(async (key: string, start: (signal: AbortSignal, onEvent: (e: StreamEvent) => void) => Promise<void>) => {
    const controller = new AbortController(); abort.current = controller; setBusy(true);
    try { await start(controller.signal, consume(key)); }
    catch (e) { if (!controller.signal.aborted) patch(key, { error: e instanceof Error ? e.message : 'AI-сервис недоступен' }); }
    finally {
      patch(key, { streaming: false, stage: undefined }); setBusy(false); abort.current = null;
      const next = queue.current.shift(); if (next) void sendRef.current(next, true); // asked while the previous answer streamed
    }
  }, [consume, patch]);

  const send = useCallback(async (text: string, shown = false) => {
    const message = text.trim(); if (!message) return;
    if (abort.current) { queue.current.push(message); setMessages(list => [...list, { key: nextKey(), role: 'user', content: message }]); return; }
    const key = nextKey();
    const question: Msg[] = shown ? [] : [{ key: nextKey(), role: 'user', content: message }];
    setMessages(list => [...list, ...question, { key, role: 'assistant', content: '', streaming: true, stage: STAGE.retrieving }]);
    const id = storage(() => sessionStorage.getItem(convKey) || undefined, undefined);
    await runStream(key, (signal, onEvent) => bot.chat(tokenRef.current, { message, conversation_id: id }, onEvent, signal));
  }, [convKey, runStream]);
  sendRef.current = send;

  // history of the current conversation, then the daily brief for interns
  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const history = await bot.messages(token, conversationId);
        if (!active) return;
        if (history.conversation_id) remember(history.conversation_id);
        setMessages(history.messages.slice(-30).map(m => ({ key: m.id, id: m.id, role: m.role, content: m.content, sources: m.sources, action: m.action, feedback: m.feedback })));
      } catch { /* empty chat is fine */ }
      if (!active) return;
      setLoaded(true);
      if (intern && storage(() => localStorage.getItem(briefKey), null) !== today()) {
        storage(() => localStorage.setItem(briefKey, today()), undefined);
        const key = nextKey();
        setMessages(list => [...list, { key, role: 'assistant', content: '', streaming: true, brief: true, stage: 'Собираю сводку на сегодня…' }]);
        void runStream(key, (signal, onEvent) => bot.brief(tokenRef.current, onEvent, signal));
      }
    })();
    return () => { active = false; abort.current?.abort(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user.id]); // once per user: a refreshed token must not restart the chat

  useEffect(() => { if (initialQuestion && loaded) { void send(initialQuestion); onQuestionUsed?.(); } }, [initialQuestion, loaded, send, onQuestionUsed]);
  useEffect(() => { bottom.current?.scrollIntoView({ block: 'end' }); }, [messages]);

  const submit = (e?: FormEvent) => { e?.preventDefault(); const text = input; setInput(''); void send(text); };
  const keydown = (e: KeyboardEvent<HTMLTextAreaElement>) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } };
  const reset = () => { abort.current?.abort(); queue.current = []; storage(() => sessionStorage.removeItem(convKey), undefined); setConversationId(undefined); setMessages([]); };
  const rate = async (m: Msg, rating: 'up' | 'down') => { if (!m.id) return; patch(m.key, { feedback: rating }); try { await bot.feedback(token, m.id, rating); } catch { patch(m.key, { feedback: m.feedback }); } };

  return <div className={compact ? 'bot-chat compact' : 'bot-chat'}>
    <div className="bot-toolbar"><span className="muted">{busy ? 'Бади думает…' : 'Ответы — по демо-базе знаний, со ссылками на источники'}</span>
      <button className="text" onClick={reset} disabled={!messages.length}>Новый диалог</button></div>
    <div className="bot-log" aria-live="polite">
      {!messages.length && loaded && <div className="bot-empty"><p>Привет{user.first_name ? `, ${user.first_name}` : ''}! Я Бади — помогу с онбордингом: отвечу по базе знаний, оформлю заявку или передам вопрос наставнику.</p>
        <div className="suggestions">{(intern ? SUGGESTIONS_INTERN : SUGGESTIONS_STAFF).map(s => <button key={s} onClick={() => void send(s)}>{s}</button>)}</div></div>}
      {messages.map(m => m.role === 'user'
        ? <p key={m.key} className="bubble mine">{m.content}</p>
        : <div key={m.key} className={m.brief ? 'bubble ai brief' : 'bubble ai'}>
          {m.brief && <span className="eyebrow">Сводка на сегодня</span>}
          {m.content ? <Markdown text={m.content} sources={m.sources}/> : m.streaming && <p className="bot-stage"><span className="bot-dots"><i/><i/><i/></span>{m.stage}</p>}
          {m.streaming && m.content && <span className="bot-caret"/>}
          {m.error && <p className="notice error">{m.error}</p>}
          {!!m.sources?.length && !m.streaming && <details className="bot-sources"><summary>Источники ({m.sources.length})</summary>
            {m.sources.map((s, i) => <div key={s.id + i}><a href={s.url} target="_blank" rel="noreferrer">[{i + 1}] {s.title}</a>{s.snippet && <blockquote>«{s.snippet}»</blockquote>}</div>)}</details>}
          {m.action && <ActionCard msg={m} token={token} onChange={change => patch(m.key, change)}/>}
          {!m.streaming && m.id && m.content && <div className="bot-rate">
            <button className={m.feedback === 'up' ? 'on' : ''} title="Полезно" onClick={() => void rate(m, 'up')}>👍</button>
            <button className={m.feedback === 'down' ? 'on' : ''} title="Не помогло" onClick={() => void rate(m, 'down')}>👎</button></div>}
        </div>)}
      <div ref={bottom}/>
    </div>
    <form className="chat-form" onSubmit={submit}>
      <textarea maxLength={2000} value={input} onChange={e => setInput(e.target.value)} onKeyDown={keydown} placeholder="Спросите Бади… (Enter — отправить)"/>
      <button disabled={!input.trim()}>Отправить</button>
    </form>
  </div>;
}
