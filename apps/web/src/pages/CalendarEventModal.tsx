import { useEffect, useState, type FormEvent } from 'react';
import type { CalendarEvent } from '@ai-buddy/client';
import { combineDateTime, dateInputValue, formatDate, parseDateInput, timeInputValue } from '../lib/calendar';

type Input = Omit<CalendarEvent, 'id'>;
type Props = {
  event: CalendarEvent;
  onClose: () => void;
  onSave: (id: string, input: Input) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
};

/** Карточка дела: сначала просмотр, «Редактировать» переключает в форму (как окно заметки). */
export function CalendarEventModal({ event, onClose, onSave, onDelete }: Props) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(event.title);
  const [description, setDescription] = useState(event.description);
  const [date, setDate] = useState(dateInputValue(event.starts_at));
  const [from, setFrom] = useState(timeInputValue(event.starts_at));
  const [to, setTo] = useState(timeInputValue(event.ends_at));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const reset = () => {
    setTitle(event.title); setDescription(event.description);
    setDate(dateInputValue(event.starts_at)); setFrom(timeInputValue(event.starts_at)); setTo(timeInputValue(event.ends_at));
    setError('');
  };
  const cancelEdit = () => { reset(); setEditing(false); };

  const save = async (e: FormEvent) => {
    e.preventDefault();
    const day = parseDateInput(date);
    if (!title.trim()) { setError('Введите название дела'); return; }
    if (!day) { setError('Укажите дату'); return; }
    if (to <= from) { setError('Окончание должно быть позже начала'); return; }
    setBusy(true); setError('');
    try {
      await onSave(event.id, { title: title.trim(), description: description.trim(), starts_at: combineDateTime(day, from), ends_at: combineDateTime(day, to) });
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось сохранить дело');
    } finally { setBusy(false); }
  };

  const drop = async () => {
    setBusy(true); setError('');
    try { await onDelete(event.id); onClose(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось удалить дело'); setBusy(false); }
  };

  const when = `${formatDate(new Date(event.starts_at))}, ${timeInputValue(event.starts_at)}–${timeInputValue(event.ends_at)}`;

  return (
    <div className="calev-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="calev-modal" role="dialog" aria-modal="true" aria-label={editing ? 'Редактирование дела' : 'Дело'}>
        <div className="calev-head">
          <span className="calev-meta">{when}</span>
          <h2>{editing ? 'Редактирование дела' : event.title}</h2>
        </div>
        {editing ? (
          <form className="calev-form" onSubmit={e => void save(e)}>
            <label>Название<input aria-label="Название дела" maxLength={120} value={title} autoFocus onChange={e => setTitle(e.target.value)} /></label>
            <div className="calev-row">
              <label>Дата<input aria-label="Дата" type="date" value={date} onChange={e => setDate(e.target.value)} /></label>
              <label>С<input aria-label="Время начала" type="time" value={from} onChange={e => setFrom(e.target.value)} /></label>
              <label>До<input aria-label="Время окончания" type="time" value={to} onChange={e => setTo(e.target.value)} /></label>
            </div>
            <label>Описание<textarea aria-label="Описание" value={description} onChange={e => setDescription(e.target.value)} /></label>
            {error && <p className="calev-error" role="alert">{error}</p>}
            <div className="calev-foot">
              <button type="button" className="calev-delete" disabled={busy} onClick={() => void drop()}>Удалить</button>
              <button type="button" className="calev-close" disabled={busy} onClick={cancelEdit}>Отмена</button>
              <button type="submit" className="calev-save" disabled={busy}>{busy ? 'Сохраняем…' : 'Сохранить'}</button>
            </div>
          </form>
        ) : (
          <div className="calev-form">
            <p className="calev-text">{event.description || 'Описания нет'}</p>
            {error && <p className="calev-error" role="alert">{error}</p>}
            <div className="calev-foot">
              <button type="button" className="calev-delete" disabled={busy} onClick={() => void drop()}>Удалить</button>
              <button type="button" className="calev-close" disabled={busy} onClick={onClose}>Закрыть</button>
              <button type="button" className="calev-save" disabled={busy} onClick={() => setEditing(true)}>Редактировать</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
