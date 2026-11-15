import { useState } from 'react';
import type { User } from '@ai-buddy/client';
import type { ChecklistsState } from '../hooks/useChecklists';
import { displayName } from '../lib/format';
import { ChecklistCard } from '../components/home/ChecklistCard';
import { MiniCalendar } from '../components/home/MiniCalendar';
import { TaskTable } from '../components/home/TaskTable';

type Props = {
  user: User;
  checklists: ChecklistsState;
  onOpenTasks: (type: 'common' | 'professional') => void;
  token?: string;
  onOpenCalendar?: (date: Date) => void;
};

export function HomePage({ user, checklists, onOpenTasks, token, onOpenCalendar }: Props) {
  const [selected, setSelected] = useState<'common' | 'professional'>('common');
  const { data, loading, error, refresh } = checklists;

  return (
    <div className="home">
      <header className="greeting">
        <h1>Привет, {displayName(user.email)}!</h1>
        <p>Пространство для организации своего времени</p>
      </header>
      {loading && !data && <p className="state-message">Загружаем задачи…</p>}
      {error && (
        <p className="state-message state-error" role="alert">
          {error} <button className="link" onClick={() => void refresh()}>Повторить</button>
        </p>
      )}
      {data && (
        <>
          <div className={`home-top is-${selected}`}>
            <ChecklistCard checklist={data.common} selected={selected === 'common'} onSelect={() => setSelected('common')} />
            <ChecklistCard
              checklist={data.professional}
              selected={selected === 'professional'}
              onSelect={() => setSelected('professional')}
            />
            <MiniCalendar token={token} onPickDate={onOpenCalendar} />
          </div>
          <TaskTable checklist={data[selected]} onOpen={() => onOpenTasks(selected)} />
        </>
      )}
    </div>
  );
}
