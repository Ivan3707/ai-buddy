import { useEffect } from 'react';

type Props = { open: boolean; onClose: () => void };

/** Заметки: шапка по макету (Заметки/Variant2). Содержимое дизайнер пока не нарисовал. */
export function NotesDrawer({ open, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer notes-drawer" role="dialog" aria-label="Заметки" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><img src="/icons/notes.svg" alt="" width={23} height={26} /></span>
          <h2>Заметки</h2>
        </header>
        <div className="drawer-body notes-body">
          <p>Здесь будут ваши заметки. Раздел в разработке.</p>
        </div>
      </aside>
    </div>
  );
}
