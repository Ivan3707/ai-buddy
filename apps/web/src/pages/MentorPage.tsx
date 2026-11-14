import { MentorDesk } from '../bot/MentorDesk';

/** Вопросы и заявки: что бот передал наставнику, AI-черновики ответов, согласование заявок и метрики эффекта. */
export function MentorPage({ token }: { token: string }) {
  return (
    <div className="home staff-page">
      <header className="greeting">
        <h1>Вопросы и заявки</h1>
        <p>Что бот передал вам: вопросы стажёров, заявки на согласование и эффект от Бади</p>
      </header>
      <MentorDesk token={token} />
    </div>
  );
}
