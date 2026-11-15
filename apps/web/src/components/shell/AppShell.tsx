import { useCallback, useEffect, useState } from 'react';
import type { User } from '@ai-buddy/client';
import { useChecklists } from '../../hooks/useChecklists';
import { useAdminAccess } from '../../hooks/useAdminAccess';
import { useStageScale } from '../../hooks/useStageScale';
import { isAdminPath } from '../../lib/adminPath';
import { overallProgress } from '../../lib/format';
import { CalendarPage } from '../../pages/CalendarPage';
import { FeedbackPage } from '../../pages/FeedbackPage';
import { HomePage } from '../../pages/HomePage';
import { TasksPage } from '../../pages/TasksPage';
import { finishOnboarding, isOnboardingPending } from '../../lib/onboarding';
import { Onboarding } from '../onboarding/Onboarding';
import { NotesDrawer } from './NotesDrawer';
import { ProfileDrawer } from './ProfileDrawer';
import { SideTabs } from './SideTabs';
import { AdminPage } from '../../pages/AdminPage';
import { Sidebar, type View } from './Sidebar';

type Props = { user: User; token: string; onLogout: () => void };

/** Рабочее пространство стажёра: меню слева, страница в центре, вкладки справа. */
export function AppShell({ user, token, onLogout }: Props) {
  const [view, setView] = useState<View>(isAdminPath() ? 'admin' : 'home');
  const [profileOpen, setProfileOpen] = useState(false);
  const [notesOpen, setNotesOpen] = useState(false);
  const [tasksChecklist, setTasksChecklist] = useState<'common' | 'professional'>('common');
  const [onboarding, setOnboarding] = useState(() => isOnboardingPending(user.id));
  const checklists = useChecklists(token);
  useStageScale();
  const isAdmin = useAdminAccess(token, user.role);
  // подсказки Бади могут открыть раздел («есть вопросы стажёров» -> «Вопросы»)
  useEffect(() => {
    const open = (event: Event) => {
      const target = String((event as CustomEvent).detail);
      if (['home', 'calendar', 'tasks'].includes(target)) setView(target as View);
    };
    window.addEventListener('ai-buddy:navigate', open);
    return () => window.removeEventListener('ai-buddy:navigate', open);
  }, []);

  // Экран обратной связи появляется после завершения обоих чек-листов (один раз за сессию браузера)
  const { data } = checklists;
  const bothDone = !!data && data.common.completion_percent >= 100 && data.professional.completion_percent >= 100;
  useEffect(() => {
    if (!bothDone) return;
    const key = `feedback-shown:${user.id}`;
    try {
      if (sessionStorage.getItem(key)) return;
      sessionStorage.setItem(key, '1');
    } catch { /* без хранилища показываем один раз за жизнь компонента */ }
    setView('feedback');
  }, [bothDone, user.id]);

  const endOnboarding = useCallback(() => {
    finishOnboarding(user.id);
    setOnboarding(false);
  }, [user.id]);
  const tourNavigate = useCallback((next: 'home' | 'tasks', checklist?: 'common' | 'professional') => {
    if (checklist) setTasksChecklist(checklist);
    setView(next);
  }, []);

  if (view === 'feedback') {
    return (
      <div className="shell">
        <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
        <FeedbackPage user={user} token={token} checklists={checklists} />
      </div>
    );
  }

  return (
    <div className="shell">
      <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
      <main className="workspace">
        {view === 'home' && <HomePage user={user} token={token} checklists={checklists} onFeedback={() => setView('feedback')} />}
        {view === 'calendar' && <CalendarPage />}
        {view === 'tasks' && <TasksPage token={token} checklists={checklists} selected={tasksChecklist} onSelect={setTasksChecklist} />}
        {view === 'admin' && (isAdmin ? <AdminPage token={token} /> : <p className="muted">Нет доступа к админ-панели.</p>)}
      </main>
      <SideTabs onProfile={() => setProfileOpen(true)} onNotes={() => setNotesOpen(true)} />
      <ProfileDrawer
        user={user}
        progress={overallProgress(checklists.data)}
        open={profileOpen}
        onClose={() => setProfileOpen(false)}
      />
      <NotesDrawer open={notesOpen} token={token} onClose={() => setNotesOpen(false)} />
      {onboarding && <Onboarding onNavigate={tourNavigate} onDone={endOnboarding} />}
    </div>
  );
}
