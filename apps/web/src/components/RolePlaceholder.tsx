import type { User } from '@ai-buddy/client';

const roleNames: Record<'buddy' | 'teamlead', string> = {
  buddy: 'Наставник',
  teamlead: 'Руководитель команды',
};

export function RolePlaceholder({ user, onLogout }: { user: User; onLogout: () => void }) {
  const role = user.role === 'teamlead' ? 'teamlead' : 'buddy';
  const name = [user.first_name, user.last_name].filter(Boolean).join(' ') || user.email;

  return (
    <main className="center" style={{ padding: '24px' }}>
      <section className="panel" style={{ width: 'min(100%, 560px)', textAlign: 'left' }}>
        <span className="eyebrow">AI·BUDDY · {roleNames[role]}</span>
        <h1 style={{ fontSize: 'clamp(30px, 5vw, 42px)', marginBottom: '16px' }}>
          Рабочее пространство в разработке
        </h1>
        <p>Здравствуйте, {name}!</p>
        <p className="muted">
          Интерфейс для роли «{roleNames[role]}» временно недоступен.
          Функциональность сервера сохранена — мы готовим обновлённую страницу.
        </p>
        <button type="button" onClick={onLogout}>Выйти из аккаунта</button>
      </section>
    </main>
  );
}
