import { useState } from 'react';
import { useCalendarEvents } from '../../hooks/useCalendarEvents';
import { WEEKDAYS, daySpan, dayMonthLabel, isSameDay, monthGrid, monthTitle, shiftMonth } from '../../lib/calendar';
import { ChevronIcon } from '../icons';

type Props = { today?: Date; token?: string; onPickDate?: (date: Date) => void };

/** Компактный календарь месяца для главной: у даты видно число дел, клик открывает календарь на этом дне. */
export function MiniCalendar({ today = new Date(), token, onPickDate }: Props) {
  const [{ year, month }, setMonth] = useState({ year: today.getFullYear(), month: today.getMonth() });
  const { events, eventsOf } = useCalendarEvents(token);
  const cells = monthGrid(year, month);
  return (
    <section className="mini-calendar" aria-label="Календарь">
      <header>
        <div>
          <h2>Календарь</h2>
          <span className="calendar-date">{monthTitle(year, month)}</span>
        </div>
        <div className="calendar-arrows">
          <button aria-label="Предыдущий месяц" onClick={() => setMonth(shiftMonth(year, month, -1))}>
            <ChevronIcon direction="left" />
          </button>
          <button aria-label="Следующий месяц" onClick={() => setMonth(shiftMonth(year, month, 1))}>
            <ChevronIcon direction="right" />
          </button>
        </div>
      </header>
      <div className="calendar-grid">
        {WEEKDAYS.map(day => <span key={day} className="calendar-weekday">{day}</span>)}
        {cells.map((cell, index) => {
          if (cell === null) return <span key={`empty-${index}`} />;
          const count = eventsOf(cell.date).length;
          const span = daySpan(events, cell.date);
          const column = index % 7;
          return (
            <button
              key={cell.day}
              type="button"
              className={`calendar-cell${span ? ` span-${span}` : ''}${span && span !== 'single' && column === 0 ? ' is-week-start' : ''}${span && span !== 'single' && column === 6 ? ' is-week-end' : ''}`}
              aria-label={`${dayMonthLabel(cell.date)}${count ? `, дел: ${count}` : ''}`}
              onClick={() => onPickDate?.(cell.date)}
            >
              <span className={`calendar-day${isSameDay(cell.date, today) ? ' is-today' : ''}${span === 'single' ? ' has-event' : ''}`}>{cell.day}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
