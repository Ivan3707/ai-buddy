export const WEEKDAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'] as const;

export type CalendarCell = { day: number; date: Date } | null;

/** Сетка месяца, неделя начинается с понедельника; пустые ячейки — null. */
export function monthGrid(year: number, month: number): CalendarCell[] {
  const first = new Date(year, month, 1);
  const offset = (first.getDay() + 6) % 7;
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const cells: CalendarCell[] = Array.from({ length: offset }, () => null);
  for (let day = 1; day <= daysInMonth; day += 1) cells.push({ day, date: new Date(year, month, day) });
  return cells;
}

export function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function shiftMonth(year: number, month: number, delta: number): { year: number; month: number } {
  const date = new Date(year, month + delta, 1);
  return { year: date.getFullYear(), month: date.getMonth() };
}

export function formatDate(date: Date): string {
  return new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' }).format(date);
}

export function monthTitle(year: number, month: number): string {
  const title = new Intl.DateTimeFormat('ru-RU', { month: 'long', year: 'numeric' }).format(new Date(year, month, 1));
  return title.charAt(0).toUpperCase() + title.slice(1);
}

/** Ключ дня для группировки событий: локальная дата без времени. */
export function dayKey(date: Date): string {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

/** «11 декабря» — для заголовка панели дел выбранного дня. */
export function dayMonthLabel(date: Date): string {
  return new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' }).format(date);
}

/** Склеивает дату и время «ЧЧ:ММ» в ISO-строку для API. */
export function combineDateTime(date: Date, time: string): string {
  const [hours, minutes] = time.split(':').map(Number);
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), hours || 0, minutes || 0).toISOString();
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Локальная дата «ГГГГ-ММ-ДД» для input[type=date]. */
export function dateInputValue(value: string | Date): string {
  const d = typeof value === 'string' ? new Date(value) : value;
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Локальное время «ЧЧ:ММ» для input[type=time]. */
export function timeInputValue(value: string): string {
  const d = new Date(value);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** «ГГГГ-ММ-ДД» -> локальная дата (null, если строка некорректна). */
export function parseDateInput(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  return m ? new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])) : null;
}

export type DaySpan ='single' | 'start' | 'mid' | 'end' | null;

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

/**
 * Как день покрыт делами: 'single' — только однодневные дела (число в розовом круге),
 * 'start' | 'mid' | 'end' — день внутри промежутка из нескольких дней (розовая плашка), null — дел нет.
 */
export function daySpan(events: { starts_at: string; ends_at: string }[], date: Date): DaySpan {
  const day = startOfDay(date);
  let result: DaySpan = null;
  for (const event of events) {
    const first = startOfDay(new Date(event.starts_at));
    const last = Math.max(first, startOfDay(new Date(event.ends_at)));
    if (day < first || day > last) continue;
    if (first === last) { result = result ?? 'single'; continue; }
    return day === first ? 'start' : day === last ? 'end' : 'mid';
  }
  return result;
}

/** День для выбора после перехода на другой месяц: сегодня, если он в этом месяце, иначе 1-е число. */
export function defaultDayOfMonth(year: number, month: number, today: Date): Date {
  return today.getFullYear() === year && today.getMonth() === month ? today : new Date(year, month, 1);
}
