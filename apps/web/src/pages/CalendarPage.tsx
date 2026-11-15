import { useState, type FormEvent } from 'react';
import { ChevronIcon } from '../components/icons';
import { useCalendarEvents } from '../hooks/useCalendarEvents';
import { WEEKDAYS, combineDateTime, defaultDayOfMonth, dayMonthLabel, isSameDay, monthGrid, monthTitle, shiftMonth } from '../lib/calendar';

type Props = { today?: Date; token?: string; initialDate?: Date };

const MAX_CHIPS = 2;

function timeLabel(value: string) {
  return new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' }).format(new Date(value));
}

/**
 * Календарь по макету calendar.png: белая вкладка с названием, розовый переключатель месяца,
 * белая карточка, слева серая панель дел выбранного дня, справа сетка дней.
 * День выбирают кликом; в панели можно добавить и удалить дело. Задачи чек-листов здесь не показываем.
 */
export function CalendarPage({ today = new Date(), token, initialDate }: Props) {
  const start = initialDate ?? today;
  const [{ year, month }, setMonth] = useState({ year: start.getFullYear(), month: start.getMonth() });
  const [selected, setSelected] = useState(start);
  const [adding, setAdding] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [from, setFrom] = useState('10:00');
  const [to, setTo] = useState('11:00');
  const [formError, setFormError] = useState('');
  const [busy, setBusy] = useState(false);
  const { events, error, add, remove, eventsOf } = useCalendarEvents(token);

  const cells = monthGrid(year, month);
  const rows = Math.ceil(cells.length / 7);
  const dayEvents = eventsOf(selected);
  const heading = isSameDay(selected, today) ? 'Дела на сегодня' : `Дела на ${dayMonthLabel(selected)}`;

  const changeMonth = (delta: number) => {
    const next = shiftMonth(year, month, delta);
    setMonth(next);
    setSelected(defaultDayOfMonth(next.year, next.month, today));
    setFormError('');
  };

  const resetForm = () => { setAdding(false); setTitle(''); setDescription(''); setFrom('10:00'); setTo('11:00'); setFormError(''); };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim()) { setFormError('Введите название дела'); return; }
    if (to <= from) { setFormError('Окончание должно быть позже начала'); return; }
    setBusy(true);
    setFormError('');
    try {
      await add({ title: title.trim(), description: description.trim(), starts_at: combineDateTime(selected, from), ends_at: combineDateTime(selected, to) });
      resetForm();
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Не удалось добавить дело');
    } finally {
      setBusy(false);
    }
  };

  const drop = async (id: string) => {
    try { await remove(id); } catch (reason) { setFormError(reason instanceof Error ? reason.message : 'Не удалось удалить дело'); }
  };

  return (
    <div className="cal-page">
      <h1 className="cal-title">Календарь</h1>
      <div className="cal-switch" role="group" aria-label="Месяц">
        <button type="button" aria-label="Предыдущий месяц" onClick={() => changeMonth(-1)}>
          <ChevronIcon direction="left" />
        </button>
        <span className="cal-month">{monthTitle(year, month).replace(/\s*г\.$/, '')}</span>
        <button type="button" aria-label="Следующий месяц" onClick={() => changeMonth(1)}>
          <ChevronIcon direction="right" />
        </button>
      </div>
      <div className="cal-card">
        <aside className="cal-today" aria-label={heading}>
          <h2>{heading}</h2>
          {error && <p className="cal-error" role="alert">{error}</p>}
          {dayEvents.length === 0 && !adding && <p className="cal-today-empty">Дел на этот день нет</p>}
          <ul>
            {dayEvents.map(event => (
              <li key={event.id}>
                <b>{timeLabel(event.starts_at)}–{timeLabel(event.ends_at)}</b>
                <span>{event.title}</span>
                {event.description && <small>{event.description}</small>}
                <button type="button" className="cal-remove" aria-label={`Удалить дело «${event.title}»`} onClick={() => void drop(event.id)}>×</button>
              </li>
            ))}
          </ul>
          {adding ? (
            <form className="cal-form" onSubmit={event => void submit(event)}>
              <input aria-label="Название дела" placeholder="Что нужно сделать" maxLength={120} value={title} autoFocus onChange={event => setTitle(event.target.value)} />
              <div className="cal-form-time">
                <label>с<input aria-label="Время начала" type="time" value={from} onChange={event => setFrom(event.target.value)} /></label>
                <label>до<input aria-label="Время окончания" type="time" value={to} onChange={event => setTo(event.target.value)} /></label>
              </div>
              <textarea aria-label="Описание" placeholder="Описание (необязательно)" rows={3} value={description} onChange={event => setDescription(event.target.value)} />
              {formError && <p className="cal-error" role="alert">{formError}</p>}
              <div className="cal-form-bar">
                <button type="button" className="cal-cancel" onClick={resetForm}>Отмена</button>
                <button type="submit" className="cal-save" disabled={busy}>{busy ? 'Сохраняем…' : 'Добавить'}</button>
              </div>
            </form>
          ) : (
            <>
              {formError && <p className="cal-error" role="alert">{formError}</p>}
              <button type="button" className="cal-add" onClick={() => setAdding(true)} disabled={!token}>+ Добавить дело</button>
            </>
          )}
        </aside>
        <div className="cal-grid" style={{ ['--cal-rows' as string]: rows }}>
          {WEEKDAYS.map(day => <span key={day} className="cal-weekday">{day}</span>)}
          {cells.map((cell, index) => {
            if (cell === null) return <span key={`empty-${index}`} className="cal-cell is-empty" />;
            const list = eventsOf(cell.date);
            const classes = ['cal-cell', isSameDay(cell.date, today) && 'is-today', isSameDay(cell.date, selected) && 'is-selected'].filter(Boolean).join(' ');
            return (
              <button
                key={cell.day}
                type="button"
                className={classes}
                aria-label={`${dayMonthLabel(cell.date)}${list.length ? `, дел: ${list.length}` : ''}`}
                aria-pressed={isSameDay(cell.date, selected)}
                onClick={() => { setSelected(cell.date); setFormError(''); }}
              >
                <b>{cell.day}</b>
                {list.slice(0, MAX_CHIPS).map(event => <i key={event.id} className="cal-chip" title={event.title}>{event.title}</i>)}
                {list.length > MAX_CHIPS && <i className="cal-chip is-more">+{list.length - MAX_CHIPS}</i>}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
