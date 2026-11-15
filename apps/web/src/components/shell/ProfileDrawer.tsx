import { useEffect } from 'react';
import type { User } from '@ai-buddy/client';
import { personName, plural, professionLabel, type OverallProgress } from '../../lib/format';

const refName = (ref: User['buddy']) => (ref ? [ref.last_name, ref.first_name].filter(Boolean).join(' ') || '—' : '—');
const tenureText = (months: number) => {
  if (months < 1) return 'до месяца';
  const years = Math.floor(months / 12);
  const rest = months % 12;
  return [years > 0 ? `${years} ${plural(years, ['год', 'года', 'лет'])}` : '', rest > 0 ? `${rest} ${plural(rest, ['месяц', 'месяца', 'месяцев'])}` : ''].filter(Boolean).join(' ');
};

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
  const fullName = personName(user).split(' ');
  const orgLine = [user.department, user.team].filter(Boolean);
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" role="dialog" aria-label="Профиль" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><img src="/icons/user.svg" alt="" width={33} height={33} /></span>
          <h2>Личный кабинет</h2>
        </header>

        <div className="drawer-body">
          <section className="person-card" aria-label="Стажёр">
            <span className="avatar" aria-hidden="true">
              {(user.first_name ?? fullName[0] ?? '').charAt(0)}
            </span>
            <h3>{fullName.map(part => <span key={part}>{part}</span>)}</h3>
          </section>

          <div className="fact-row">
            {user.age_years != null && (
              <section className="fact-card"><h3>Возраст:</h3><p>{user.age_years} {plural(user.age_years, ['год', 'года', 'лет'])}</p></section>
            )}
            {user.tenure_months != null && (
              <section className="fact-card"><h3>Стаж:</h3><p>{tenureText(user.tenure_months)}</p></section>
            )}
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
              <span style={{ width: progress.percent > 0 ? `max(55px, ${progress.percent}%)` : 0 }} />
            </div>
            <div className="progress-edges"><span>Начало стажировки</span><span>Конец стажировки</span></div>
          </section>

          <section className="about-card" aria-label="О себе">
            <h3>О себе</h3>
            <dl>
              <div><dt>Направление стажировки:</dt><dd>{professionLabel(user.profession)}</dd></div>
              <div><dt>Руководитель:</dt><dd>{refName(user.teamlead)}</dd></div>
              <div><dt>Наставник:</dt><dd>{refName(user.buddy)}</dd></div>
            </dl>
            {orgLine.length > 0 && <p className="about-org">{orgLine[0]}{orgLine[1] && <><br />{orgLine[1]}</>}</p>}
          </section>
        </div>
      </aside>
    </div>
  );
}
