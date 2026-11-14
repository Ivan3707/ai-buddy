import { useState } from 'react';
import type { User } from '@ai-buddy/client';
import { useChecklists } from '../../hooks/useChecklists';
import { overallProgress } from '../../lib/format';
import { CalendarPage } from '../../pages/CalendarPage';
import { FeedbackPage } from '../../pages/FeedbackPage';
import { HomePage } from '../../pages/HomePage';
import { TasksPage } from '../../pages/TasksPage';
import { Assistant } from '../Assistant';
import { ProfileDrawer } from './ProfileDrawer';
import { SideTabs } from './SideTabs';
import { Sidebar, type View } from './Sidebar';

type Props = { user: User; token: string; onLogout: () => void };

/** Рабочее пространство стажёра: меню слева, страница в центре, вкладки справа. */
export function AppShell({ user, token, onLogout }: Props) {
  const [view, setView] = useState<View>('home');
  const [profileOpen, setProfileOpen] = useState(false);
  const checklists = useChecklists(token);

  if (view === 'feedback') {
    return (
      <div className="shell">
        <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
        <FeedbackPage user={user} />
        <Assistant token={token} />
      </div>
    );
  }

  return (
    <div className="shell">
      <Sidebar view={view} onNavigate={setView} onLogout={onLogout} />
      <main className="workspace">
        {view === 'home' && <HomePage user={user} checklists={checklists} onOpenTasks={() => setView('tasks')} />}
        {view === 'calendar' && <CalendarPage />}
        {view === 'tasks' && <TasksPage token={token} checklists={checklists} />}
      </main>
      <SideTabs onProfile={() => setProfileOpen(true)} onFeedback={() => setView('feedback')} />
      <ProfileDrawer
        user={user}
        progress={overallProgress(checklists.data)}
        open={profileOpen}
        onClose={() => setProfileOpen(false)}
      />
      <Assistant token={token} />
    </div>
  );
}
