import { useCallback, useEffect, useMemo, useState } from 'react';
import { createEvent, deleteEvent, listEvents, updateEvent, type CalendarEvent } from '@ai-buddy/client';
import { dayKey } from '../lib/calendar';

type NewEvent = Omit<CalendarEvent, 'id'>;

/** События календаря пользователя: список, добавление, удаление и разбивка по дням. Без токена ничего не грузит. */
export function useCalendarEvents(token?: string) {
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!token) return;
    try {
      setEvents(await listEvents('', token));
      setError('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось загрузить календарь');
    }
  }, [token]);

  useEffect(() => { void load(); }, [load]);

  const add = useCallback(async (input: NewEvent) => {
    if (!token) throw new Error('Нужно войти, чтобы добавлять дела');
    await createEvent('', token, input);
    await load();
  }, [token, load]);

  const update = useCallback(async (id: string, input: NewEvent) => {
    if (!token) throw new Error('Нужно войти, чтобы менять дела');
    await updateEvent('', token, id, input);
    await load();
  }, [token, load]);

  const remove = useCallback(async (id: string) => {
    if (!token) return;
    await deleteEvent('', token, id);
    await load();
  }, [token, load]);

  const byDay = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>();
    for (const event of events) {
      const key = dayKey(new Date(event.starts_at));
      map.set(key, [...(map.get(key) ?? []), event]);
    }
    for (const list of map.values()) list.sort((a, b) => a.starts_at.localeCompare(b.starts_at));
    return map;
  }, [events]);

  const eventsOf = useCallback((date: Date) => byDay.get(dayKey(date)) ?? [], [byDay]);

  return { events, error, add, update, remove, eventsOf };
}
