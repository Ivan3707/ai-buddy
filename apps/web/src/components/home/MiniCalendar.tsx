import { useState } from 'react';
import { WEEKDAYS, isSameDay, monthGrid, monthTitle, shiftMonth } from '../../lib/calendar';
import { ChevronIcon } from '../icons';

type Props = { today?: Date };

/** Компактный календарь месяца для главной страницы. */
export function MiniCalendar({ today = new Date() }: Props) {
  const [{ year, month }, setMonth] = useState({ year: today.getFullYear(), month: today.getMonth() });
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
        {cells.map((cell, index) => cell === null
          ? <span key={`empty-${index}`} />
          : <span key={cell.day} className={`calendar-day${isSameDay(cell.date, today) ? ' is-today' : ''}`}>{cell.day}</span>)}
      </div>
    </section>
  );
}
