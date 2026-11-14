import { useEffect } from 'react';
import type { User } from '@ai-buddy/client';
import { internshipDuration, type OverallProgress } from '../../lib/format';
import { PROFILE_STUB } from '../../lib/stubs';
import { UserIcon } from '../icons';

type Props = {
  user: User;
  progress: OverallProgress;
  open: boolean;
  onClose: () => void;
};

/** Личный кабинет: панель справа с данными о стажёре (макет personal.png). */
export function ProfileDrawer({ user, progress, open, onClose }: Props) {
  // Крестика нет: панель закрывается по Esc и по клику на затемнённый фон
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  const profile = PROFILE_STUB;
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" role="dialog" aria-label="Профиль" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><UserIcon width={26} height={26} /></span>
          <h2>Личный кабинет</h2>
        </header>

        <div className="drawer-body">
          <section className="person-card" aria-label="Стажёр">
            <span className="avatar" aria-hidden="true">
              {profile.photoUrl ? <img src={profile.photoUrl} alt="" /> : profile.fullName[1]?.charAt(0)}
            </span>
            <h3>{profile.fullName.map(part => <span key={part}>{part}</span>)}</h3>
          </section>

          <div className="fact-row">
            <section className="fact-card"><h3>Возраст:</h3><p>{profile.age} лет</p></section>
            <section className="fact-card"><h3>Стаж:</h3><p>{internshipDuration(user.created_at)}</p></section>
          </div>

          <section className="progress-card" aria-label="Прогресс стажировки">
            <h3>Прогресс стажировки</h3>
            <strong className="progress-percent">{progress.percent} %</strong>
            <span className="progress-hint">в работе: {progress.active}</span>
            <div className="progress-line"><span>Общий прогресс</span><span>{progress.completed}/{progress.total}</span></div>
            <div
              className="progress-bar"
              role="progressbar"
              aria-label="Общий прогресс стажировки"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={progress.percent}
            >
              <span style={{ width: `${progress.percent}%` }} />
            </div>
            <div className="progress-edges"><span>Начало стажировки</span><span>Конец стажировки</span></div>
          </section>

          <section className="about-card" aria-label="О себе">
            <h3>О себе</h3>
            <dl>
              <div><dt>Направление стажировки:</dt><dd>{user.profession === 'backend' ? 'Бэкенд-разработчик' : user.profession ?? '—'}</dd></div>
              <div><dt>Руководитель:</dt><dd>{profile.manager}</dd></div>
              <div><dt>Наставник:</dt><dd>{profile.mentor}</dd></div>
            </dl>
            <p className="about-org">{profile.department}<br />{profile.unit}</p>
          </section>
        </div>
      </aside>
    </div>
  );
}
