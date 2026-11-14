import type { Checklist, ChecklistItem, ChecklistsResponse, User } from '@ai-buddy/client';

export const internUser: User = {
  id: 'u-intern',
  email: 'spike.novikov@example.com',
  role: 'intern',
  profession: 'backend',
  created_at: '2026-11-14T10:00:00Z',
};

export const buddyUser: User = { ...internUser, id: 'u-buddy', email: 'buddy@example.com', role: 'buddy', profession: null };

let counter = 0;
export function item(over: Partial<ChecklistItem> = {}): ChecklistItem {
  counter += 1;
  return {
    id: `item-${counter}`,
    title: `Задача ${counter}`,
    description: `Описание задачи ${counter}`,
    expected_result: null,
    requires_review: false,
    estimated_minutes: 30,
    position: counter,
    status: 'unavailable',
    completed_at: null,
    ...over,
  };
}

export function checklist(over: Partial<Checklist> = {}): Checklist {
  const items = [
    item({ title: 'Первая задача', status: 'completed', completed_at: '2026-11-14T11:00:00Z' }),
    item({ title: 'Вторая задача', status: 'active', expected_result: 'Результат есть' }),
    item({ title: 'Третья задача', status: 'unavailable' }),
  ];
  return {
    id: 'cl-common',
    name: 'Адаптация в команде (Demo v1)',
    type: 'common',
    profession: null,
    stages: [{ id: 'st-1', title: 'Знакомство', week_start: 1, week_end: 2, goal: 'Освоиться', expected_result: 'Готов', position: 1, items }],
    total_items: 3,
    completed_items: 1,
    completion_percent: 33.3,
    ...over,
  };
}

export function checklistsResponse(): ChecklistsResponse {
  return {
    common: checklist(),
    professional: checklist({
      id: 'cl-prof',
      name: 'Backend Developer — адаптация (Demo v1)',
      type: 'professional',
      profession: 'backend',
      completed_items: 0,
      completion_percent: 0,
      stages: [{
        id: 'st-p', title: 'Погружение', week_start: 3, week_end: 3, goal: 'Погрузиться', expected_result: 'Ок', position: 1,
        items: [item({ title: 'Доступ к репозиторию', status: 'active', requires_review: true })],
      }],
    }),
  };
}
