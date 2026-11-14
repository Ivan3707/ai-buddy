import { useCallback, useEffect, useState } from 'react';
import { createNote, deleteNote, listNotes, updateNote, type Note } from '@ai-buddy/client';
import { formatDateTime } from '../../lib/format';

type Props = { open: boolean; token: string; onClose: () => void };
type Draft = { id?: string; title: string; content: string };

/** Заметки справа: список своих заметок, создание, правка и удаление (те же заметки, что во вкладке «Заметки» Бади). */
export function NotesDrawer({ open, token, onClose }: Props) {
  const [notes, setNotes] = useState<Note[]>([]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [wide, setWide] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try { setNotes(await listNotes('', token)); setError(''); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось загрузить заметки'); }
  }, [token]);

  useEffect(() => { if (open) { void load(); setDraft(null); } }, [open, load]);
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const save = async () => {
    if (!draft || !draft.title.trim()) { setError('Введите заголовок заметки'); return; }
    setBusy(true); setError('');
    try {
      const input = { title: draft.title.trim(), content: draft.content };
      if (draft.id) await updateNote('', token, draft.id, input); else await createNote('', token, input);
      setDraft(null);
      await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось сохранить'); }
    finally { setBusy(false); }
  };
  const remove = async () => {
    if (!draft?.id) return;
    setBusy(true); setError('');
    try { await deleteNote('', token, draft.id); setDraft(null); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось удалить'); }
    finally { setBusy(false); }
  };

  if (!open) return null;
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className={`drawer notes-drawer${wide ? ' is-wide' : ''}`} role="dialog" aria-label="Заметки" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><img src="/icons/notes.svg" alt="" width={23} height={26} /></span>
          <h2>Заметки</h2>
        </header>
        <div className="drawer-body notes-body">
          <div className="notes-content">
            {error && <p className="notes-error" role="alert">{error}</p>}
            {draft ? (
              <form className="notes-editor" onSubmit={event => { event.preventDefault(); void save(); }}>
                <input aria-label="Заголовок" placeholder="Заголовок" maxLength={120} value={draft.title} autoFocus
                  onChange={event => setDraft({ ...draft, title: event.target.value })} />
                <textarea aria-label="Текст заметки" placeholder="Текст заметки" rows={8} value={draft.content}
                  onChange={event => setDraft({ ...draft, content: event.target.value })} />
                <div className="notes-editor-bar">
                  {draft.id && <button type="button" className="notes-link danger" disabled={busy} onClick={() => void remove()}>Удалить</button>}
                  <button type="button" className="notes-link" disabled={busy} onClick={() => setDraft(null)}>Отмена</button>
                  <button type="submit" className="notes-save" disabled={busy}>{busy ? 'Сохраняем…' : 'Сохранить'}</button>
                </div>
              </form>
            ) : notes.length === 0 ? (
              <p className="notes-empty">Заметок пока нет. Нажмите «+», чтобы добавить первую.</p>
            ) : (
              <ul className="notes-list">
                {notes.map(note => (
                  <li key={note.id}>
                    <button type="button" onClick={() => setDraft({ id: note.id, title: note.title, content: note.content })}>
                      <b>{note.title}</b>
                      {note.content && <span>{note.content}</span>}
                      <time>{formatDateTime(note.updated_at)}</time>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div className="notes-actions">
            <button type="button" className="notes-action" aria-label="Добавить заметку" onClick={() => setDraft({ title: '', content: '' })}>
              <svg width="28" height="28" viewBox="0 0 28 28" fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round" aria-hidden="true">
                <path d="M14 4v20M4 14h20" />
              </svg>
            </button>
            <button type="button" className="notes-action notes-action-open" aria-label={wide ? 'Свернуть панель' : 'Развернуть панель'} onClick={() => setWide(value => !value)}>
              <svg width="23" height="23" viewBox="0 0 23 23" fill="none" aria-hidden="true">
                <path d="M16.1 0C15.4649 0 14.95 0.514872 14.95 1.15C14.95 1.78513 15.4649 2.3 16.1 2.3H19.0737L8.96183 12.4118C8.51272 12.8609 8.51272 13.5891 8.96183 14.0382C9.41093 14.4873 10.1391 14.4873 10.5882 14.0382L20.7 3.92635V6.9C20.7 7.53513 21.2149 8.05 21.85 8.05C22.4851 8.05 23 7.53513 23 6.9V1.15C23 0.514872 22.4851 0 21.85 0H16.1ZM2.3 3.45C2.3 2.81487 2.81487 2.3 3.45 2.3H9.2C9.83513 2.3 10.35 1.78513 10.35 1.15C10.35 0.514873 9.83513 0 9.2 0H3.45C1.54462 0 0 1.54462 0 3.45V19.55C0 21.4554 1.54462 23 3.45 23H19.55C21.4554 23 23 21.4554 23 19.55V13.7233C23 13.0882 22.4851 12.5733 21.85 12.5733C21.2149 12.5733 20.7 13.0882 20.7 13.7233V19.55C20.7 20.1851 20.1851 20.7 19.55 20.7H3.45C2.81487 20.7 2.3 20.1851 2.3 19.55V3.45Z" fill="white" />
              </svg>
            </button>
          </div>
        </div>
      </aside>
    </div>
  );
}
