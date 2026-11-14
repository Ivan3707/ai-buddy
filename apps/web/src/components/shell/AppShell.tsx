import { useEffect, useState } from 'react';
import type { User } from '@ai-buddy/client';
import { useChecklists } from '../../hooks/useChecklists';
import { useStageScale } from '../../hooks/useStageScale';
import { overallProgress } from '../../lib/format';
import { CalendarPage } from '../../pages/CalendarPage';
import { FeedbackPage } from '../../pages/FeedbackPage';
import { HomePage } from '../../pages/HomePage';
import { TasksPage } from '../../pages/TasksPage';
import { BuddyInfoPage } from '../../bot/BuddyPages';
import { NotesDrawer } from './NotesDrawer';
import { ProfileDrawer } from './ProfileDrawer';
import { SideTabs } from './SideTabs';
import { Sidebar, type View } from './Sidebar';

type Props = { user: User; token: string; onLogout: () => void };

/** Рабочее пространство стажёра: меню слева, страница в центре, вкладки справа. */
export function AppShell({ user, token, onLogout }: Props) {
  const [view, setView] = useState<View>('home');
  const [profileOpen, setProfileOpen] = useState(false);
  const [notesOpen, setNotesOpen] = useState(false);
  const [tasksChecklist, setTasksChecklist] = useState<'common' | 'professional'>('common');
  const checklists = useChecklists(token);
  useStageScale();
  // подсказки Бади могут открыть раздел («есть вопросы стажёров» -> «Вопросы»)
  useEffect(() => {
    const open = (event: Event) => {
      const target = String((event as CustomEvent).detail);
      if (['home', 'calendar', 'tasks', 'buddy'].includes(target)) setView(target as View);
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

  if (view === 'feedback') {
    return (
      <div className="shell">
        <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
        <FeedbackPage user={user} />
      </div>
    );
  }

  return (
    <div className="shell">
      <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
      <main className="workspace">
        {view === 'home' && <HomePage user={user} checklists={checklists} onOpenTasks={type => { setTasksChecklist(type); setView('tasks'); }} />}
        {view === 'calendar' && <CalendarPage />}
        {view === 'tasks' && <TasksPage token={token} checklists={checklists} selected={tasksChecklist} onSelect={setTasksChecklist} />}
        {view === 'buddy' && <BuddyInfoPage user={user} />}
      </main>
      <SideTabs onProfile={() => setProfileOpen(true)} onNotes={() => setNotesOpen(true)} />
      <ProfileDrawer
        user={user}
        progress={overallProgress(checklists.data)}
        open={profileOpen}
        onClose={() => setProfileOpen(false)}
      />
      <NotesDrawer open={notesOpen} onClose={() => setNotesOpen(false)} />
    </div>
  );
}
