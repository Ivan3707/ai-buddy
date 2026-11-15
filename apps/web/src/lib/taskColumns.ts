import type { ChecklistItem } from '@ai-buddy/client';

/** Колонка канбана. «На проверке» и «Завершено» — это разные взгляды на статус `completed`/`verified` из бэкенда. */
export type Column = 'unavailable' | 'active' | 'review' | 'done';

export const COLUMN_TITLES: Record<Column, string> = {
  unavailable: 'Недоступно',
  active: 'Активно',
  review: 'На проверке',
  done: 'Завершено',
};

/**
 * Задача, отправленная наставнику, остаётся `completed`, пока он её не принял (`verified`).
 * Задача без проверки после завершения сразу попадает в «Завершено».
 */
export function columnOf(item: Pick<ChecklistItem, 'status' | 'requires_review'>): Column {
  if (item.status === 'unavailable' || item.status === 'active') return item.status;
  if (item.status === 'completed' && item.requires_review) return 'review';
  return 'done';
}

/** Подпись кнопки следующего шага: зависит от того, нужна ли проверка. */
export function nextStepLabel(item: Pick<ChecklistItem, 'requires_review'>): string {
  return item.requires_review ? 'На проверку' : 'Завершить';
}
