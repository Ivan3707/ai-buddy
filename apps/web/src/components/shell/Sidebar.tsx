export type View = 'home' | 'calendar' | 'tasks' | 'feedback' | 'team' | 'mentor' | 'admin';
export type NavItem = { view: Exclude<View, 'feedback'>; label: string };

export const NAV_ITEMS: NavItem[] = [
  { view: 'home', label: 'Главная' },
  { view: 'calendar', label: 'Календарь' },
  { view: 'tasks', label: 'Задачи' },
];

type Props = { view: View; onNavigate: (view: View) => void; onLogout: () => void; items?: NavItem[] };

/** Меню слева. Пункты задаются снаружи: у стажёра и у наставника они разные, оформление одно. */
export function Sidebar({ view, onNavigate, onLogout, items = NAV_ITEMS }: Props) {
  return (
    <aside className="sidebar">
      <span className="lang">RU</span>
      <nav aria-label="Основная навигация">
        {items.map(item => (
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
      <button className="sidebar-exit" aria-label="Выйти" onClick={onLogout}>
        <img src="/icons/exit.svg" alt="" width={27} height={28} />
      </button>
    </aside>
  );
}
