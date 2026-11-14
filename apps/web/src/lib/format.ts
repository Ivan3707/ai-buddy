import type { Checklist, ChecklistItem, ChecklistStage, ChecklistsResponse, User } from '@ai-buddy/client';

/** Имя для приветствия: часть email до «@», с заглавной буквы. */
export function displayName(email: string): string {
  const local = email.split('@')[0]?.trim() ?? '';
  const first = local.split(/[._-]/)[0] ?? '';
  if (!first) return 'друг';
  return first.charAt(0).toUpperCase() + first.slice(1);
}

export function roleLabel(role: User['role']): string {
  return role === 'buddy' ? 'Buddy' : 'Стажёр';
}

export function professionLabel(profession: User['profession']): string {
  switch (profession) {
    case 'backend': return 'Backend-разработчик';
    case 'frontend': return 'Frontend-разработчик';
    case 'ml': return 'ML-инженер';
    default: return '—';
  }
}

/** «1-2 неделя» или «3 неделя», если этап длится одну неделю. */
export function weekLabel(stage: Pick<ChecklistStage, 'week_start' | 'week_end'>): string {
  return stage.week_start === stage.week_end
    ? `${stage.week_start} неделя`
    : `${stage.week_start}-${stage.week_end} неделя`;
}

export const isDone = (item: Pick<ChecklistItem, 'status'>): boolean =>
  item.status === 'completed' || item.status === 'verified';

/** Текущий этап — первый, где остались невыполненные задачи; иначе последний. */
export function currentStage(checklist: Checklist): ChecklistStage | undefined {
  return checklist.stages.find(stage => stage.items.some(item => !isDone(item)))
    ?? checklist.stages[checklist.stages.length - 1];
}

export type OverallProgress = { completed: number; total: number; active: number; percent: number };

/** Сводный прогресс по обоим чек-листам. */
export function overallProgress(data: ChecklistsResponse | undefined): OverallProgress {
  const lists = data ? [data.common, data.professional] : [];
  const items = lists.flatMap(list => list.stages.flatMap(stage => stage.items));
  const total = items.length;
  const completed = items.filter(isDone).length;
  const active = items.filter(entry => entry.status === 'active').length;
  return { completed, total, active, percent: total === 0 ? 0 : Math.round((completed / total) * 100) };
}

/** Русское склонение: plural(3, ['месяц', 'месяца', 'месяцев']) → «месяца». */
export function plural(count: number, forms: [string, string, string]): string {
  const n = Math.abs(count) % 100;
  const last = n % 10;
  if (n > 10 && n < 20) return forms[2];
  if (last === 1) return forms[0];
  if (last >= 2 && last <= 4) return forms[1];
  return forms[2];
}

/** Стаж по дате создания аккаунта: «меньше месяца», «3 месяца», «1 год 2 месяца». */
export function internshipDuration(createdAt: string, now: Date = new Date()): string {
  const start = new Date(createdAt);
  let months = (now.getFullYear() - start.getFullYear()) * 12 + now.getMonth() - start.getMonth();
  if (now.getDate() < start.getDate()) months -= 1;
  if (months < 1) return 'меньше месяца';
  const years = Math.floor(months / 12);
  const rest = months % 12;
  const parts: string[] = [];
  if (years > 0) parts.push(`${years} ${plural(years, ['год', 'года', 'лет'])}`);
  if (rest > 0) parts.push(`${rest} ${plural(rest, ['месяц', 'месяца', 'месяцев'])}`);
  return parts.join(' ');
}

/** Убирает техническую пометку версии вроде «(Demo v1)» из названия чек-листа. */
export function cleanChecklistName(name: string): string {
  return name.replace(/\s*\(Demo[^)]*\)\s*$/i, '').trim();
}

export function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
