import { MiniCalendar } from '../components/home/MiniCalendar';

/** Заглушка: макет страницы календаря ещё не готов. */
export function CalendarPage() {
  return (
    <div className="calendar-page">
      <h1 className="page-heading">Календарь</h1>
      <div className="calendar-page-body"><MiniCalendar /></div>
    </div>
  );
}
