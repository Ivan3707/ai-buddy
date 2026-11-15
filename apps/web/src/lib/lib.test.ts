import { afterEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../test/fixtures';
import { WEEKDAYS, formatDate, isSameDay, monthGrid, monthTitle, shiftMonth } from './calendar';
import { cleanChecklistName, currentStage, displayName, formatDateTime, internshipDuration, isDone, overallProgress, plural, professionLabel, roleLabel, weekLabel } from './format';

describe('format', () => {
  it('displayName берёт первую часть email с заглавной буквы', () => {
    expect(displayName('spike.novikov@example.com')).toBe('Spike');
    expect(displayName('ivan_petrov@x.ru')).toBe('Ivan');
    expect(displayName('@x.ru')).toBe('друг');
  });

  it('подписи ролей и профессий', () => {
    expect(roleLabel('buddy')).toBe('Buddy');
    expect(roleLabel('intern')).toBe('Стажёр');
    expect(professionLabel('backend')).toBe('Backend-разработчик');
    expect(professionLabel('frontend')).toBe('Frontend-разработчик');
    expect(professionLabel('ml')).toBe('ML-инженер');
    expect(professionLabel(null)).toBe('—');
  });

  it('weekLabel различает диапазон и одну неделю', () => {
    expect(weekLabel({ week_start: 1, week_end: 2 })).toBe('1-2 неделя');
    expect(weekLabel({ week_start: 3, week_end: 3 })).toBe('3 неделя');
  });

  it('isDone считает выполненными completed и verified', () => {
    expect(isDone(item({ status: 'completed' }))).toBe(true);
    expect(isDone(item({ status: 'verified' }))).toBe(true);
    expect(isDone(item({ status: 'active' }))).toBe(false);
  });

  it('currentStage выбирает первый незавершённый этап, иначе последний', () => {
    const done = item({ status: 'verified' });
    const open = item({ status: 'active' });
    const base = checklist();
    const stage = (id: string, items: typeof base.stages[0]['items']) => ({ ...base.stages[0]!, id, items });
    expect(currentStage({ ...base, stages: [stage('a', [done]), stage('b', [open])] })?.id).toBe('b');
    expect(currentStage({ ...base, stages: [stage('a', [done]), stage('b', [done])] })?.id).toBe('b');
    expect(currentStage({ ...base, stages: [] })).toBeUndefined();
  });

  it('overallProgress считает сводный прогресс по двум чек-листам', () => {
    expect(overallProgress(undefined)).toEqual({ completed: 0, total: 0, active: 0, percent: 0 });
    const data = checklistsResponse();
    // common: completed/active/unavailable, professional: одна активная задача
    expect(overallProgress(data)).toEqual({ completed: 1, total: 4, active: 2, percent: 25 });
  });

  it('plural склоняет по-русски', () => {
    const forms: [string, string, string] = ['месяц', 'месяца', 'месяцев'];
    expect([1, 2, 5, 11, 12, 21, 22, 25, 111].map(n => plural(n, forms)))
      .toEqual(['месяц', 'месяца', 'месяцев', 'месяцев', 'месяцев', 'месяц', 'месяца', 'месяцев', 'месяцев']);
  });

  it('internshipDuration считает стаж по дате создания', () => {
    const now = new Date(2026, 10, 14);
    expect(internshipDuration('2026-11-01T10:00:00', now)).toBe('до месяца');
    expect(internshipDuration('2026-10-20T10:00:00', now)).toBe('до месяца'); // ещё не прошёл месяц
    expect(internshipDuration('2026-08-14T10:00:00', now)).toBe('3 месяца');
    expect(internshipDuration('2025-09-10T10:00:00', now)).toBe('1 год 2 месяца');
    expect(internshipDuration('2024-11-01T10:00:00', now)).toBe('2 года');
  });

  it('cleanChecklistName убирает пометку версии', () => {
    expect(cleanChecklistName('Адаптация в команде (Demo v1)')).toBe('Адаптация в команде');
    expect(cleanChecklistName('Без пометки')).toBe('Без пометки');
  });

  it('formatDateTime возвращает локализованную строку', () => {
    expect(formatDateTime('2026-11-14T10:30:00Z')).toMatch(/2026/);
  });
});

describe('calendar', () => {
  it('monthGrid начинает неделю с понедельника', () => {
    const grid = monthGrid(2026, 10); // ноябрь 2026 начинается с воскресенья
    expect(grid.slice(0, 6).every(cell => cell === null)).toBe(true);
    expect(grid[6]?.day).toBe(1);
    expect(grid.filter(Boolean)).toHaveLength(30);
  });

  it('isSameDay, shiftMonth, форматирование', () => {
    expect(isSameDay(new Date(2026, 10, 14, 1), new Date(2026, 10, 14, 23))).toBe(true);
    expect(isSameDay(new Date(2026, 10, 14), new Date(2026, 10, 15))).toBe(false);
    expect(shiftMonth(2026, 11, 1)).toEqual({ year: 2027, month: 0 });
    expect(shiftMonth(2026, 0, -1)).toEqual({ year: 2025, month: 11 });
    expect(formatDate(new Date(2026, 10, 14))).toBe('14.11.2026');
    expect(monthTitle(2026, 10)).toMatch(/^Ноябрь/);
    expect(WEEKDAYS).toHaveLength(7);
  });
});

import { daySpan, defaultDayOfMonth } from './calendar';

describe('calendar spans and month change', () => {
  const ev = (a: string, b: string) => ({ starts_at: new Date(a).toISOString(), ends_at: new Date(b).toISOString() });
  it('classifies single and multi-day coverage', () => {
    const events = [ev('2026-11-10T10:00', '2026-11-10T11:00'), ev('2026-11-12T10:00', '2026-11-14T11:00')];
    expect(daySpan(events, new Date(2026, 10, 10))).toBe('single');
    expect(daySpan(events, new Date(2026, 10, 11))).toBeNull();
    expect(daySpan(events, new Date(2026, 10, 12))).toBe('start');
    expect(daySpan(events, new Date(2026, 10, 13))).toBe('mid');
    expect(daySpan(events, new Date(2026, 10, 14))).toBe('end');
  });
  it('selects a day inside the shown month', () => {
    const today = new Date(2026, 10, 15);
    expect(defaultDayOfMonth(2026, 11, today)).toEqual(new Date(2026, 11, 1));
    expect(defaultDayOfMonth(2026, 10, today)).toEqual(today);
  });
});
