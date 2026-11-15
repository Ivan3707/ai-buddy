import { useState } from 'react';
import type { User } from '@ai-buddy/client';
import type { ChecklistsState } from '../hooks/useChecklists';
import { displayName } from '../lib/format';
import { ChecklistCard } from '../components/home/ChecklistCard';
import { MiniCalendar } from '../components/home/MiniCalendar';
import { TaskTable } from '../components/home/TaskTable';
import { TaskModal } from '../components/tasks/TaskModal';
import { advanceItem } from '../lib/advance';

type Props = {
  user: User;
  token: string;
  checklists: ChecklistsState;
  onFeedback: () => void;
  onOpenCalendar?: (date: Date) => void;
};

export function HomePage({ user, token, checklists, onFeedback, onOpenCalendar }: Props) {
  const [selected, setSelected] = useState<'common' | 'professional'>('common');
  const { data, loading, error, refresh } = checklists;
  const [openID, setOpenID] = useState('');
  const [busy, setBusy] = useState(false);
  const cards = data ? [data.common, data.professional].flatMap(list => list.stages.flatMap(stage => stage.items.map(item => ({ item, stage, list })))) : [];
  const opened = cards.find(card => card.item.id === openID);
  const allDone = !!data && data.common.completion_percent >= 100 && data.professional.completion_percent >= 100;

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
      {allDone && (
        <section className="feedback-banner" aria-label="Обратная связь">
          <div><b>Вы прошли оба чек-листа!</b><span>Расскажите, что было полезным, а что стоит улучшить.</span></div>
          <button type="button" className="pill pill-black pill-sm" onClick={onFeedback}>Оставить обратную связь</button>
        </section>
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
          <TaskTable checklist={data[selected]} managerName={user.teamlead ? [user.teamlead.last_name, user.teamlead.first_name].filter(Boolean).join(' ') : undefined} onOpen={item => setOpenID(item.id)} />
          {opened && (
            <TaskModal
              item={opened.item}
              stage={opened.stage}
              checklistName={opened.list.type === 'common' ? 'Общий' : 'Профессиональный'}
              token={token}
              busy={busy}
              onClose={() => setOpenID('')}
              onAdvance={async result => {
                setBusy(true);
                try { await advanceItem(token, opened.item, result); await refresh(); } finally { setBusy(false); }
              }}
            />
          )}
        </>
      )}
    </div>
  );
}
