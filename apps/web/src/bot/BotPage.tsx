import { useState } from 'react';
import type { User } from '@ai-buddy/client';
import { BotChat } from './BotChat';
import { MentorDesk } from './MentorDesk';
import { Mascot } from './BotWidget';
import './bot.css';

/** /chat: the intern gets the full chat; buddy and teamlead get the mentor desk plus the chat. */
export function BotPage({ token, user }: { token: string; user: User }) {
  const staff = user.role !== 'intern';
  const [tab, setTab] = useState<'desk' | 'chat'>(staff ? 'desk' : 'chat');
  return <main className="bot-page">
    <header className="bot-page-head">
      <div className="bot-page-face"><Mascot mood="talking"/></div>
      <div><span className="eyebrow">{staff ? 'Кабинет наставника' : 'AI-наставник'}</span><h1>Бади</h1>
        <p className="muted">{staff
          ? 'Что бот передал вам: вопросы стажёров с AI-черновиком ответа, заявки на согласование и эффект — сколько вопросов он закрыл сам.'
          : 'Отвечает по базе знаний со ссылками, честно говорит о противоречиях, оформляет заявки после твоего подтверждения и передаёт вопрос наставнику, если ответа нет. Данные демо.'}</p></div>
    </header>
    {staff && <div className="desk-tabs bot-page-tabs"><button className={tab === 'desk' ? '' : 'ghost'} onClick={() => setTab('desk')}>Кабинет</button><button className={tab === 'chat' ? '' : 'ghost'} onClick={() => setTab('chat')}>Чат с Бади</button></div>}
    {tab === 'desk' ? <MentorDesk token={token}/> : <section className="panel bot-page-chat"><BotChat token={token} user={user}/></section>}
  </main>;
}
