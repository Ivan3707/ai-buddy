import type { ChecklistItem } from '@ai-buddy/client';

/**
 * Колонка канбана. «Задачи» (`unavailable`) — ещё не начатые: сверху доступная к старту, ниже заблокированные.
 * «На проверке» и «Завершено» — это разные взгляды на статус `completed`/`verified` из бэкенда.
 */
export type Column = 'unavailable' | 'active' | 'review' | 'done';

export const COLUMN_TITLES: Record<Column, string> = {
  unavailable: 'Задачи',
  active: 'Активно',
  review: 'На проверке',
  done: 'Завершено',
};

/**
 * Задача открыта, но стажёр ещё не взял её в работу: бэкенд держит её `active` со `started: false`.
 * На доске она стоит первой в колонке «Задачи» с кнопкой «В работу». Без поля (старые данные) — уже начата.
 */
export function isAvailable(item: Pick<ChecklistItem, 'status' | 'started'>): boolean {
  return item.status === 'active' && item.started === false;
}

/**
 * Задача, отправленная наставнику, остаётся `completed`, пока он её не принял (`verified`).
 * Задача без проверки после завершения сразу попадает в «Завершено».
 */
export function columnOf(item: Pick<ChecklistItem, 'status' | 'requires_review' | 'started'>): Column {
  if (item.status === 'unavailable' || isAvailable(item)) return 'unavailable';
  if (item.status === 'active') return 'active';
  if (item.status === 'completed' && item.requires_review) return 'review';
  return 'done';
}

/** Подпись состояния задачи для людей: в колонке «Задачи» различаем доступную и заблокированную. */
export function statusLabel(item: Pick<ChecklistItem, 'status' | 'requires_review' | 'started'>): string {
  if (item.status === 'unavailable') return 'Недоступно';
  if (isAvailable(item)) return 'Можно начать';
  return COLUMN_TITLES[columnOf(item)];
}

/** Подпись кнопки следующего шага: зависит от того, нужна ли проверка. */
export function nextStepLabel(item: Pick<ChecklistItem, 'requires_review'>): string {
  return item.requires_review ? 'На проверку' : 'Завершить';
}
