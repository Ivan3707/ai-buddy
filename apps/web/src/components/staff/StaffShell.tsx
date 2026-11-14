import { useEffect, useState } from 'react';
import type { User } from '@ai-buddy/client';
import { BuddyInfoPage } from '../../bot/BuddyPages';
import { useAdminAccess } from '../../hooks/useAdminAccess';
import { isAdminPath } from '../../lib/adminPath';
import { useStageScale } from '../../hooks/useStageScale';
import { AdminPage } from '../../pages/AdminPage';
import { CalendarPage } from '../../pages/CalendarPage';
import { MentorPage } from '../../pages/MentorPage';
import { StaffHomePage } from '../../pages/StaffHomePage';
import { StaffTeamPage } from '../../pages/StaffTeamPage';
import { NotesDrawer } from '../shell/NotesDrawer';
import { SideTabs } from '../shell/SideTabs';
import { Sidebar, type NavItem, type View } from '../shell/Sidebar';
import { StaffProfileDrawer } from './StaffProfileDrawer';

const STAFF_ITEMS: NavItem[] = [
  { view: 'home', label: 'Главная' },
  { view: 'team', label: 'Команда' },
  { view: 'mentor', label: 'Вопросы' },
  { view: 'calendar', label: 'Календарь' },
  { view: 'buddy', label: 'Бади' },
];

type Props ={ user: User; token: string; onLogout: () => void };

/** Рабочее пространство наставника и руководителя: то же меню, сцена и вкладки справа, что у стажёра. */
export function StaffShell({ user, token, onLogout }: Props) {
  const [view, setView] = useState<View>(isAdminPath() ? 'admin' : 'home');
  const [profileOpen, setProfileOpen] = useState(false);
  const [notesOpen, setNotesOpen] = useState(false);
  useStageScale();
  // подсказки Бади могут открыть раздел («есть вопросы стажёров» -> «Вопросы»)
  useEffect(() => {
    const open = (event: Event) => {
      const target = String((event as CustomEvent).detail);
      if (['home', 'team', 'mentor', 'calendar', 'buddy'].includes(target)) setView(target as View);
    };
    window.addEventListener('ai-buddy:navigate', open);
    return () => window.removeEventListener('ai-buddy:navigate', open);
  }, []);
  // Админка открывается по адресу /admin: руководителю всегда, остальным только при ADMIN_OPEN_FOR_ALL на бэкенде (демо). Права проверяет бэкенд.
  const isAdmin = useAdminAccess(token, user.role);

  return (
    <div className="shell">
      <Sidebar view={view} items={STAFF_ITEMS} onNavigate={setView} onLogout={onLogout} />
      <main className="workspace staff">
        {view === 'home' && <StaffHomePage user={user} token={token} onNavigate={setView} />}
        {view === 'team' && <StaffTeamPage user={user} token={token} />}
        {view === 'mentor' && <MentorPage token={token} />}
        {view === 'calendar' && <CalendarPage />}
        {view === 'buddy' && <BuddyInfoPage user={user} />}
        {view === 'admin' && (isAdmin ? <AdminPage token={token} /> : <p className="muted">Нет доступа к админ-панели.</p>)}
      </main>
      <SideTabs onProfile={() => setProfileOpen(true)} onNotes={() => setNotesOpen(true)} />
      <StaffProfileDrawer user={user} open={profileOpen} onClose={() => setProfileOpen(false)} />
      <NotesDrawer open={notesOpen} onClose={() => setNotesOpen(false)} />
    </div>
  );
}
