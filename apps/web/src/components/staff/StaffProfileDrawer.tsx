import { useEffect } from 'react';
import type { User } from '@ai-buddy/client';
import { personName, staffRoleLabel } from '../../lib/format';

type Props = { user: User; open: boolean; onClose: () => void };

/** Личный кабинет наставника и руководителя: та же панель справа, что у стажёра, но с реальными данными из профиля. */
export function StaffProfileDrawer({ user, open, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  const parts = personName(user).split(' ');
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" role="dialog" aria-label="Профиль" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><img src="/icons/user.svg" alt="" width={33} height={33} /></span>
          <h2>Личный кабинет</h2>
        </header>
        <div className="drawer-body">
          <section className="person-card" aria-label="Сотрудник">
            <span className="avatar" aria-hidden="true">{(parts[0] || user.email).charAt(0).toUpperCase()}</span>
            <h3>{parts.map(part => <span key={part}>{part}</span>)}</h3>
          </section>
          <div className="fact-row">
            <section className="fact-card"><h3>Роль:</h3><p>{staffRoleLabel(user.role)}</p></section>
            <section className="fact-card"><h3>Возраст:</h3><p>{user.age_years ? `${user.age_years} лет` : '—'}</p></section>
          </div>
          <section className="about-card" aria-label="О себе">
            <h3>О себе</h3>
            <dl>
              <div><dt>Почта:</dt><dd>{user.email}</dd></div>
              <div><dt>Команда:</dt><dd>{user.team ?? '—'}</dd></div>
              {user.telegram_username && <div><dt>Telegram:</dt><dd>@{user.telegram_username.replace(/^@/, '')}</dd></div>}
            </dl>
            <p className="about-org">{user.department ?? ''}</p>
          </section>
        </div>
      </aside>
    </div>
  );
}
