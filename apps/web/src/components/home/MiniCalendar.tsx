import { useState } from 'react';
import { WEEKDAYS, formatDate, isSameDay, monthGrid, monthTitle, shiftMonth } from '../../lib/calendar';
import { ChevronIcon } from '../icons';

type Props = { today?: Date };

/** Компактный календарь месяца для главной страницы. */
export function MiniCalendar({ today = new Date() }: Props) {
  const [{ year, month }, setMonth] = useState({ year: today.getFullYear(), month: today.getMonth() });
  const cells = monthGrid(year, month);
  const isCurrentMonth = year === today.getFullYear() && month === today.getMonth();
  return (
    <section className="mini-calendar" aria-label="Календарь">
      <header>
        <div>
          <h2>Календарь</h2>
          <span className="calendar-date">{isCurrentMonth ? formatDate(today) : monthTitle(year, month)}</span>
        </div>
        <div className="calendar-arrows">
          <button aria-label="Предыдущий месяц" onClick={() => setMonth(shiftMonth(year, month, -1))}>
            <ChevronIcon direction="left" width={14} height={14} />
          </button>
          <button aria-label="Следующий месяц" onClick={() => setMonth(shiftMonth(year, month, 1))}>
            <ChevronIcon direction="right" width={14} height={14} />
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
