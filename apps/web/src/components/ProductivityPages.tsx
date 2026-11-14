import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { createEvent, createNote, deleteEvent, deleteNote, listEvents, listNotes, updateEvent, updateNote, type CalendarEvent, type Note } from '@ai-buddy/client';
import { Modal } from './Modal';
import { Page } from './Pages';

const errorText = (reason: unknown) => reason instanceof Error ? reason.message : 'Не удалось выполнить запрос';
const localInput = (value: string) => { const date = new Date(value); const offset = date.getTimezoneOffset() * 60000; return new Date(date.getTime() - offset).toISOString().slice(0, 16); };
const formatDate = (value: string) => new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));

function inline(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  const pattern = /(\*\*([^*]+)\*\*|\*([^*]+)\*|\[([^\]]+)\]\(([^)]+)\))/g;
  let at = 0; let match: RegExpExecArray | null;
  while ((match = pattern.exec(text))) {
    if (match.index > at) parts.push(text.slice(at, match.index));
    if (match[2]) parts.push(<strong key={match.index}>{match[2]}</strong>);
    else if (match[3]) parts.push(<em key={match.index}>{match[3]}</em>);
    else {
      const href = match[5];
      parts.push(/^https?:\/\//i.test(href) ? <a key={match.index} href={href} target="_blank" rel="noreferrer">{match[4]}</a> : match[4]);
    }
    at = pattern.lastIndex;
  }
  if (at < text.length) parts.push(text.slice(at));
  return parts;
}

function MarkdownPreview({ text }: { text: string }) {
  if (!text.trim()) return <p className="muted">Добавьте текст, чтобы увидеть предпросмотр.</p>;
  return <div className="markdown-preview">{text.split('\n').map((line, index) => {
    if (line.startsWith('### ')) return <h4 key={index}>{inline(line.slice(4))}</h4>;
    if (line.startsWith('## ')) return <h3 key={index}>{inline(line.slice(3))}</h3>;
    if (line.startsWith('# ')) return <h2 key={index}>{inline(line.slice(2))}</h2>;
    if (line.startsWith('- ')) return <p key={index}>• {inline(line.slice(2))}</p>;
    return <p key={index}>{inline(line) || '\u00a0'}</p>;
  })}</div>;
}

export function NotesPage({ token }: { token: string }) {
  const [notes, setNotes] = useState<Note[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [draft, setDraft] = useState({ title: '', content: '' });
  const [search, setSearch] = useState('');
  const [preview, setPreview] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const current = notes.find(note => note.id === selected);
  const dirty = current ? current.title !== draft.title || current.content !== draft.content : Boolean(draft.title || draft.content);
  const load = useCallback(async () => { try { setNotes(await listNotes('', token)); setError(''); } catch (reason) { setError(errorText(reason)); } finally { setLoading(false); } }, [token]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => { const guard = (event: BeforeUnloadEvent) => { if (dirty) event.preventDefault(); }; addEventListener('beforeunload', guard); return () => removeEventListener('beforeunload', guard); }, [dirty]);
  const choose = (note: Note | null) => { if (dirty && !window.confirm('Есть несохранённые изменения. Продолжить?')) return; setSelected(note?.id ?? null); setDraft({ title: note?.title ?? '', content: note?.content ?? '' }); setPreview(false); setError(''); };
  const save = async () => { if (!draft.title.trim()) { setError('Укажите заголовок'); return; } setBusy(true); try { const note = current ? await updateNote('', token, current.id, draft) : await createNote('', token, draft); await load(); setSelected(note.id); setDraft({ title: note.title, content: note.content }); } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); } };
  const remove = async () => { if (!current || !window.confirm(`Удалить заметку «${current.title}»?`)) return; setBusy(true); try { await deleteNote('', token, current.id); setSelected(null); setDraft({ title: '', content: '' }); await load(); } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); } };
  const filtered = useMemo(() => notes.filter(note => `${note.title} ${note.content}`.toLocaleLowerCase().includes(search.toLocaleLowerCase())), [notes, search]);
  return <Page title="Заметки" eyebrow="Личное пространство" subtitle="Быстрые записи и Markdown без лишних инструментов."><div className="notes-workspace"><aside className="panel notes-list"><div className="section-head"><h2>Мои заметки</h2><button onClick={() => choose(null)}>+</button></div><label>Поиск<input value={search} onChange={event => setSearch(event.target.value)} placeholder="Название или текст"/></label>{loading ? <p>Загружаем…</p> : filtered.length ? filtered.map(note => <button key={note.id} className={selected === note.id ? 'selected' : ''} onClick={() => choose(note)}><b>{note.title}</b><small>{formatDate(note.updated_at)}</small></button>) : <p className="muted">Заметок не найдено.</p>}</aside><section className="panel notes-editor"><label>Заголовок<input maxLength={160} value={draft.title} onChange={event => setDraft({...draft,title:event.target.value})} placeholder="Новая заметка"/></label><div className="tab-bar"><button className={!preview ? 'selected' : ''} onClick={() => setPreview(false)}>Редактор</button><button className={preview ? 'selected' : ''} onClick={() => setPreview(true)}>Предпросмотр</button></div>{preview ? <MarkdownPreview text={draft.content}/> : <textarea className="markdown-input" maxLength={8000} value={draft.content} onChange={event => setDraft({...draft,content:event.target.value})} placeholder="# Заголовок&#10;Текст заметки…"/>}<div className="section-head"><span className="muted">{dirty ? 'Есть несохранённые изменения' : current ? 'Сохранено' : 'Новая заметка'}</span><div>{current && <button className="ghost" disabled={busy} onClick={() => void remove()}>Удалить</button>} <button disabled={busy || !dirty} onClick={() => void save()}>Сохранить</button></div></div>{error && <p className="notice error" role="alert">{error}</p>}</section></div></Page>;
}

const emptyEvent = { title: '', description: '', starts_at: '', ends_at: '' };
export function CalendarPage({ token }: { token: string }) {
  const [month, setMonth] = useState(() => new Date(new Date().getFullYear(), new Date().getMonth(), 1));
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [draft, setDraft] = useState(emptyEvent);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => { try { setEvents(await listEvents('', token)); setError(''); } catch (reason) { setError(errorText(reason)); } finally { setLoading(false); } }, [token]);
  useEffect(() => { void load(); }, [load]);
  const start = (day: Date) => { const at = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 10); setSelected('new'); setDraft({ ...emptyEvent, starts_at: localInput(at.toISOString()), ends_at: localInput(new Date(at.getTime() + 3600000).toISOString()) }); setError(''); };
  const open = (event: CalendarEvent) => { setSelected(event.id); setDraft({ title: event.title, description: event.description, starts_at: localInput(event.starts_at), ends_at: localInput(event.ends_at) }); setError(''); };
  const save = async (submit: FormEvent) => { submit.preventDefault(); const startAt = new Date(draft.starts_at); const endAt = new Date(draft.ends_at); if (!(endAt > startAt)) { setError('Окончание должно быть позже начала'); return; } setBusy(true); try { const body = { title: draft.title, description: draft.description, starts_at: startAt.toISOString(), ends_at: endAt.toISOString() }; if (selected && selected !== 'new') await updateEvent('', token, selected, body); else await createEvent('', token, body); setSelected(null); await load(); } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); } };
  const remove = async () => { if (!selected || selected === 'new' || !window.confirm('Удалить событие?')) return; setBusy(true); try { await deleteEvent('', token, selected); setSelected(null); await load(); } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); } };
  const offset = (new Date(month.getFullYear(), month.getMonth(), 1).getDay() + 6) % 7;
  const days = Array.from({length: offset + new Date(month.getFullYear(), month.getMonth()+1, 0).getDate()}, (_, index) => index < offset ? null : new Date(month.getFullYear(), month.getMonth(), index-offset+1));
  const key = (date: Date) => `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
  return <Page title="Календарь" eyebrow="Планирование" subtitle="Личные события в вашем часовом поясе."><div className="panel"><div className="calendar-toolbar"><button className="ghost" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth()-1, 1))}>←</button><h2>{new Intl.DateTimeFormat('ru-RU', {month:'long',year:'numeric'}).format(month)}</h2><button className="ghost" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth()+1, 1))}>→</button></div>{error && !selected && <p className="notice error">{error}</p>}{loading ? <p>Загружаем события…</p> : <div className="calendar-grid">{['Пн','Вт','Ср','Чт','Пт','Сб','Вс'].map(day => <b className="calendar-weekday" key={day}>{day}</b>)}{days.map((day,index) => <div className="calendar-day" key={index}>{day && <><button className="calendar-date" onClick={() => start(day)} aria-label={`Создать событие ${day.toLocaleDateString('ru-RU')}`}>{day.getDate()}</button>{events.filter(event => key(new Date(event.starts_at)) === key(day)).map(event => <button className="calendar-event" key={event.id} onClick={() => open(event)}>{event.title}</button>)}</>}</div>)}</div>}</div>{selected && <Modal title={selected === 'new' ? 'Новое событие' : 'Событие'} onClose={() => setSelected(null)}><form onSubmit={save}><label>Название<input required value={draft.title} onChange={event => setDraft({...draft,title:event.target.value})}/></label><label>Описание<textarea value={draft.description} onChange={event => setDraft({...draft,description:event.target.value})}/></label><label>Начало<input required type="datetime-local" value={draft.starts_at} onChange={event => setDraft({...draft,starts_at:event.target.value})}/></label><label>Окончание<input required type="datetime-local" value={draft.ends_at} onChange={event => setDraft({...draft,ends_at:event.target.value})}/></label>{error && <p className="notice error">{error}</p>}<div className="section-head">{selected !== 'new' && <button type="button" className="ghost" onClick={() => void remove()} disabled={busy}>Удалить</button>}<button disabled={busy}>Сохранить</button></div></form></Modal>}</Page>;
}
