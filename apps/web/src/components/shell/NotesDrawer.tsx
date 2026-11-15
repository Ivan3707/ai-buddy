import { useCallback, useEffect, useState } from 'react';
import { createNote, deleteNote, listNotes, updateNote, type Note } from '@ai-buddy/client';
import { formatDateTime } from '../../lib/format';
import { PencilIcon } from '../icons';

type Props = { open: boolean; token: string; onClose: () => void };
type Draft = { id?: string; title: string; content: string; mode: 'view' | 'edit' };

/** Заметки справа: список своих заметок, создание, правка и удаление (те же заметки, что во вкладке «Заметки» Бади). */
export function NotesDrawer({ open, token, onClose }: Props) {
  const [notes, setNotes] = useState<Note[]>([]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try { setNotes(await listNotes('', token)); setError(''); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось загрузить заметки'); }
  }, [token]);

  useEffect(() => { if (open) { void load(); setDraft(null); } }, [open, load]);
  useEffect(() => {
    if (!open) return;
    // Esc закрывает сначала окно заметки, потом саму панель
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') { if (draft) setDraft(null); else onClose(); } };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose, draft]);

  const save = async () => {
    if (!draft || !draft.title.trim()) { setError('Введите заголовок заметки'); return; }
    setBusy(true); setError('');
    try {
      const input = { title: draft.title.trim(), content: draft.content };
      if (draft.id) await updateNote('', token, draft.id, input); else await createNote('', token, input);
      if (draft.id) setDraft({ ...draft, title: input.title, mode: 'view' }); else setDraft(null);
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
    <div className="drawer-backdrop notes-backdrop" onClick={onClose}>
      <aside className="drawer notes-drawer" role="dialog" aria-label="Заметки" onClick={event => event.stopPropagation()}>
        <header className="drawer-header">
          <span className="drawer-icon"><img src="/icons/notes.svg" alt="" width={23} height={26} /></span>
          <h2>Заметки</h2>
        </header>
        <div className="drawer-body notes-body">
          <div className="notes-content">
            {error && <p className="notes-error" role="alert">{error}</p>}
            {notes.length === 0 ? (
              <p className="notes-empty">Заметок пока нет. Нажмите «+», чтобы добавить первую.</p>
            ) : (
              <ul className="notes-list">
                {notes.map(note => (
                  <li key={note.id}>
                    <button type="button" onClick={() => setDraft({ id: note.id, title: note.title, content: note.content, mode: 'view' })}>
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
            <button type="button" className="notes-action" aria-label="Добавить заметку" onClick={() => setDraft({ title: '', content: '', mode: 'edit' })}>
              <svg width="28" height="28" viewBox="0 0 28 28" fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round" aria-hidden="true">
                <path d="M14 4v20M4 14h20" />
              </svg>
            </button>
          </div>
        </div>
      </aside>
      {draft && (
        <div className="note-backdrop" onClick={event => { event.stopPropagation(); setDraft(null); }}>
          <section className="note-modal" role="dialog" aria-modal="true" aria-label={draft.id ? 'Заметка' : 'Новая заметка'} onClick={event => event.stopPropagation()}>
            <header className="note-modal-head">
              <div>
                <span className="note-modal-meta">{draft.id ? `Изменена ${formatDateTime(notes.find(note => note.id === draft.id)?.updated_at ?? new Date().toISOString())}` : 'Новая заметка'}</span>
                <h2>{draft.title.trim() || 'Без названия'}</h2>
              </div>
            </header>
            {draft.mode === 'view' ? (
              <div className="note-modal-form note-modal-view">
                {error && <p className="note-modal-error" role="alert">{error}</p>}
                <p className="note-modal-text">{draft.content || 'Текста нет'}</p>
                <div className="note-modal-foot">
                  <button type="button" className="note-modal-delete" disabled={busy} onClick={() => void remove()}>Удалить</button>
                  <button type="button" className="note-modal-close" disabled={busy} onClick={() => setDraft(null)}>Закрыть</button>
                  <button type="button" className="note-modal-save note-modal-edit" disabled={busy} onClick={() => setDraft({ ...draft, mode: 'edit' })}>
                    <PencilIcon width={18} height={18} />Редактировать
                  </button>
                </div>
              </div>
            ) : (
              <form className="note-modal-form" onSubmit={event => { event.preventDefault(); void save(); }}>
                {error && <p className="note-modal-error" role="alert">{error}</p>}
                <label>Заголовок
                  <input aria-label="Заголовок" placeholder="О чём заметка" maxLength={120} value={draft.title} autoFocus
                    onChange={event => setDraft({ ...draft, title: event.target.value })} />
                </label>
                <label>Текст заметки
                  <textarea aria-label="Текст заметки" placeholder="Что важно запомнить" rows={9} value={draft.content}
                    onChange={event => setDraft({ ...draft, content: event.target.value })} />
                </label>
                <div className="note-modal-foot">
                  <button type="button" className="note-modal-close" disabled={busy}
                    onClick={() => { setError(''); const orig = notes.find(note => note.id === draft.id); if (draft.id && orig) setDraft({ id: orig.id, title: orig.title, content: orig.content, mode: 'view' }); else setDraft(null); }}>Отмена</button>
                  <button type="submit" className="note-modal-save" disabled={busy}>{busy ? 'Сохраняем…' : 'Сохранить'}</button>
                </div>
              </form>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
