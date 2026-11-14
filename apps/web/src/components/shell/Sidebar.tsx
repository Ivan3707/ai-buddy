import { LocationIcon } from '../icons';

export type View = 'home' | 'calendar' | 'tasks' | 'feedback';

export const NAV_ITEMS: { view: Exclude<View, 'feedback'>; label: string }[] = [
  { view: 'home', label: 'Главная' },
  { view: 'calendar', label: 'Календарь' },
  { view: 'tasks', label: 'Задачи' },
];

type Props = { view: View; onNavigate: (view: View) => void };

export function Sidebar({ view, onNavigate }: Props) {
  return (
    <aside className="sidebar">
      <span className="lang">RU</span>
      <nav aria-label="Основная навигация">
        {NAV_ITEMS.map(item => (
          <button
            key={item.view}
            className={`nav-link${view === item.view ? ' is-active' : ''}`}
            aria-current={view === item.view ? 'page' : undefined}
            onClick={() => onNavigate(item.view)}
          >
            {item.label}
          </button>
        ))}
      </nav>
      <LocationIcon className="sidebar-pin" />
    </aside>
  );
}
