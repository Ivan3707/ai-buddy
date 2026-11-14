import { useState } from 'react';
import { ChevronIcon } from '../components/icons';
import { WEEKDAYS, isSameDay, monthGrid, monthTitle, shiftMonth } from '../lib/calendar';

type Props = { today?: Date };

/** Набросок страницы календаря: макет дизайнера пока содержит только тёмный кадр 1920×1080. */
export function CalendarPage({ today = new Date() }: Props) {
  const [{ year, month }, setMonth] = useState({ year: today.getFullYear(), month: today.getMonth() });
  const cells = monthGrid(year, month);
  return (
    <div className="calendar-page">
      <header className="calendar-page-head">
        <div>
          <h1>Календарь</h1>
          <span className="calendar-page-month">{monthTitle(year, month)}</span>
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
      <div className="calendar-page-grid">
        {WEEKDAYS.map(day => <span key={day} className="calendar-page-weekday">{day}</span>)}
        {cells.map((cell, index) => cell === null
          ? <span key={`empty-${index}`} className="calendar-page-cell is-empty" />
          : (
            <span key={cell.day} className={`calendar-page-cell${isSameDay(cell.date, today) ? ' is-today' : ''}`}>
              <b>{cell.day}</b>
            </span>
          ))}
      </div>
    </div>
  );
}
