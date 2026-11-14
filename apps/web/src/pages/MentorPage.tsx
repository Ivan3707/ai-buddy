import type { User } from '@ai-buddy/client';
import { MentorDesk } from '../bot/MentorDesk';
import { RequestsDesk } from '../bot/RequestsDesk';

/** Наставнику: вопросы, переданные ботом, и AI-черновики ответов. Руководителю: заявки стажёров на согласование. */
export function MentorPage({ user, token }: { user: User; token: string }) {
  const lead = user.role === 'teamlead';
  return (
    <div className="home staff-page">
      <header className="greeting">
        <h1>{lead ? 'Заявки' : 'Вопросы'}</h1>
        <p>{lead ? 'Заявки стажёров на отгул, отпуск и доступ: согласование открывается в корпоративном портале' : 'Что бот передал вам: вопросы стажёров и эффект от Бади'}</p>
      </header>
      {lead ? <RequestsDesk token={token} /> : <MentorDesk token={token} />}
    </div>
  );
}
