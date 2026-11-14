import { useCallback, useEffect, useRef, useState } from 'react';
import { getChecklists, type User } from '@ai-buddy/client';
import { bot, ownQuestions, REQUEST_STATUS, type Hint } from './api';
import { BotChat } from './BotChat';
import './bot.css';

// Floating Бади in the bottom-right corner of every page (except the full chat page). It opens the compact chat
// and speaks first: a hint for a task the intern is stuck on (backend hint worker + AI /hint), a request status
// change, a mentor's answer, or — for staff — new escalated questions.

type Nudge = { id: string; text: string; tone: 'hint' | 'happy' | 'info'; hint?: Hint; ask?: string; go?: string };

export function Mascot({ mood = 'idle' }: { mood?: 'idle' | 'talking' | 'happy' }) {
  return <svg viewBox="0 0 64 64" width="100%" height="100%" aria-hidden="true" className={`mascot ${mood}`}>
    <line x1="32" y1="6" x2="32" y2="14" stroke="#154d3b" strokeWidth="3" strokeLinecap="round"/>
    <circle className="mascot-antenna" cx="32" cy="6" r="4" fill="#e87a47"/>
    <rect x="10" y="14" width="44" height="36" rx="14" fill="#a8d7bd" stroke="#154d3b" strokeWidth="3"/>
    <rect x="17" y="22" width="30" height="18" rx="9" fill="#fff"/>
    <circle className="mascot-eye" cx="26" cy="31" r="3.4" fill="#154d3b"/><circle className="mascot-eye" cx="38" cy="31" r="3.4" fill="#154d3b"/>
    <path d={mood === 'happy' ? 'M25 43 q7 6 14 0' : 'M27 44 q5 3 10 0'} stroke="#154d3b" strokeWidth="2.6" fill="none" strokeLinecap="round"/>
    <rect x="4" y="27" width="6" height="12" rx="3" fill="#154d3b"/><rect x="54" y="27" width="6" height="12" rx="3" fill="#154d3b"/>
    <rect x="22" y="50" width="20" height="8" rx="4" fill="#154d3b"/>
  </svg>;
}

function storage<T>(fn: () => T, fallback: T): T { try { return fn(); } catch { return fallback; } }

export function BotWidget({ token, user, onNavigate }: { token: string; user: User; onNavigate: (path: string) => void }) {
  const [open, setOpen] = useState(false);
  const [nudges, setNudges] = useState<Nudge[]>([]);
  const [question, setQuestion] = useState<string | undefined>();
  const titles = useRef<Map<string, string>>(new Map());
  const intern = user.role === 'intern';
  const seenKey = `ai-buddy-seen:${user.id}`;

  const push = useCallback((n: Nudge) => setNudges(list => list.some(x => x.id === n.id) ? list : [...list, n].slice(-3)), []);
  const dismiss = (id: string) => setNudges(list => list.filter(n => n.id !== id));

  const poll = useCallback(async () => {
    const seen: Record<string, string> = storage(() => JSON.parse(localStorage.getItem(seenKey) || '{}'), {});
    const first = !Object.keys(seen).length;
    try {
      if (intern) {
        const [hints, requests, questions] = await Promise.all([bot.hints(token).catch(() => []), bot.requests(token), ownQuestions(token)]);
        for (const h of hints.filter(h => !h.read_at)) {
          if (!titles.current.size) {
            const lists = await getChecklists('', token).catch(() => null);
            for (const c of lists ? [lists.common, lists.professional] : []) for (const st of c?.stages ?? []) for (const it of st.items) titles.current.set(it.id, it.title);
          }
          const title = titles.current.get(h.item_id);
          push({ id: `hint-${h.id}`, tone: 'hint', hint: h, ask: title ? `Помоги с задачей «${title}»: с чего начать?` : undefined,
            text: title ? `Задача «${title}» идёт дольше плана. У меня есть подсказка:` : 'У меня есть подсказка по текущей задаче:' });
        }
        for (const r of requests) {
          const key = `req-${r.id}`;
          if (!first && seen[key] && seen[key] !== r.status && r.status !== 'pending_approval')
            push({ id: `${key}-${r.status}`, tone: r.status === 'approved' ? 'happy' : 'info', text: `${r.status === 'approved' ? '🎉 ' : ''}Заявка ${r.number}: ${REQUEST_STATUS[r.status]}.` });
          seen[key] = r.status;
        }
        for (const q of questions) {
          const key = `q-${q.id}`;
          if (!first && seen[key] === 'open' && q.status === 'answered') push({ id: `${key}-answered`, tone: 'happy', text: 'Наставник ответил на твой вопрос — ответ в чате.' });
          seen[key] = q.status;
        }
      } else {
        const open = (await bot.questions(token, 'open')).length;
        const pending = (await bot.staffRequests(token)).filter(r => r.request.status === 'pending_approval').length;
        const key = `staff-${open}-${pending}`;
        if ((open || pending) && seen.staff !== key)
          push({ id: key, tone: 'info', go: '/chat', text: `От бота: ${open ? `вопросов стажёров — ${open}` : ''}${open && pending ? ', ' : ''}${pending ? `заявок на согласование — ${pending}` : ''}.` });
        seen.staff = key;
      }
      seen.init = '1';
      storage(() => localStorage.setItem(seenKey, JSON.stringify(seen)), undefined);
    } catch { /* offline or token refresh in progress: try next time */ }
  }, [intern, push, seenKey, token]);

  useEffect(() => { void poll(); const id = setInterval(() => void poll(), 20000); return () => clearInterval(id); }, [poll]);

  const readHint = async (n: Nudge) => { dismiss(n.id); if (n.hint) { try { await bot.readHint(token, n.hint.id); } catch { /* ignore */ } } };
  const discuss = (n: Nudge) => { void readHint(n); setQuestion(n.ask); setOpen(true); };
  const mood = open ? 'talking' : nudges.some(n => n.tone === 'happy') ? 'happy' : 'idle';

  return <div className="bot-widget">
    {!open && nudges.map(n => <div key={n.id} className={`bot-nudge ${n.tone}`} role="status">
      <button className="bot-nudge-close" aria-label="Скрыть" onClick={() => void readHint(n)}>×</button>
      <p>{n.text}</p>
      {n.hint && <p className="bot-nudge-hint">{n.hint.text}</p>}
      <div className="bot-nudge-actions">
        {n.ask && <button onClick={() => discuss(n)}>Обсудить с Бади</button>}
        {n.go && <button onClick={() => { dismiss(n.id); onNavigate(n.go!); }}>Открыть</button>}
        {!n.ask && !n.go && <button onClick={() => { dismiss(n.id); setOpen(true); }}>Открыть чат</button>}
        {n.hint && <button className="ghost" onClick={() => void readHint(n)}>Понятно</button>}
      </div>
    </div>)}
    {open && <section className="bot-popup panel" aria-label="Чат с Бади">
      <header><div className="bot-popup-face"><Mascot mood="talking"/></div><div><b>Бади</b><small className="muted">AI-наставник · демо</small></div>
        <button className="text" title="Открыть на всю страницу" onClick={() => { setOpen(false); onNavigate('/chat'); }}>⤢</button>
        <button className="text" title="Свернуть" onClick={() => setOpen(false)}>✕</button></header>
      <BotChat token={token} user={user} compact initialQuestion={question} onQuestionUsed={() => setQuestion(undefined)}/>
    </section>}
    <button className={`bot-fab ${nudges.length && !open ? 'has-news' : ''}`} aria-label={open ? 'Свернуть чат' : 'Открыть чат с Бади'} onClick={() => setOpen(!open)}>
      <Mascot mood={mood}/>{nudges.length > 0 && !open && <span className="bot-badge">{nudges.length}</span>}
    </button>
  </div>;
}
