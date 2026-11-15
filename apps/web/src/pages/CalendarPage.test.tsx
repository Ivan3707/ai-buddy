import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CalendarPage } from './CalendarPage';

const client = vi.hoisted(() => ({
  listEvents: vi.fn(async (): Promise<unknown[]> => []),
  createEvent: vi.fn(async () => ({})),
  updateEvent: vi.fn(async () => ({})),
  deleteEvent: vi.fn(async () => undefined),
}));
vi.mock('@ai-buddy/client', () => client);

describe('CalendarPage', () => {
  beforeEach(() => vi.clearAllMocks());

  it('создаёт дело в показанном месяце, а не в месяце ранее выбранного дня', async () => {
    const user = userEvent.setup();
    render(<CalendarPage today={new Date(2026, 10, 15)} token="t" />);
    await user.click(screen.getByRole('button', { name: 'Следующий месяц' }));
    expect(screen.getByRole('complementary', { name: 'Дела на 1 декабря' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '+ Добавить дело' }));
    await user.type(screen.getByLabelText('Название дела'), 'Встреча');
    await user.click(screen.getByRole('button', { name: 'Добавить' }));
    await waitFor(() => expect(client.createEvent).toHaveBeenCalled());
    const payload = (client.createEvent.mock.calls[0] as unknown[])[2] as { starts_at: string };
    expect(new Date(payload.starts_at).getMonth()).toBe(11);
    expect(new Date(payload.starts_at).getDate()).toBe(1);
  });

  const sample = { id: 'e1', title: 'Встреча', description: 'Обсудить план', starts_at: new Date(2026, 10, 15, 10, 0).toISOString(), ends_at: new Date(2026, 10, 15, 11, 0).toISOString() };

  it('клик по делу открывает карточку; редактирование сохраняется через updateEvent', async () => {
    client.listEvents.mockResolvedValue([sample]);
    const user = userEvent.setup();
    render(<CalendarPage today={new Date(2026, 10, 15)} token="t" />);
    await user.click(await screen.findByRole('button', { name: 'Открыть дело «Встреча»' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('Обсудить план');
    expect(dialog).toHaveTextContent('15.11.2026');
    await user.click(screen.getByRole('button', { name: 'Редактировать' }));
    const input = screen.getByLabelText('Название дела');
    await user.clear(input);
    await user.type(input, 'Созвон');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(client.updateEvent).toHaveBeenCalled());
    const args = client.updateEvent.mock.calls[0] as unknown[];
    expect(args[2]).toBe('e1');
    expect((args[3] as { title: string }).title).toBe('Созвон');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('клик по чипу открывает карточку, отмена правки не вызывает API, удаление зовёт deleteEvent', async () => {
    client.listEvents.mockResolvedValue([sample]);
    const user = userEvent.setup();
    render(<CalendarPage today={new Date(2026, 10, 15)} token="t" />);
    const chips = await screen.findAllByTitle('Встреча');
    await user.click(chips[0]);
    await user.click(screen.getByRole('button', { name: 'Редактировать' }));
    await user.click(screen.getByRole('button', { name: 'Отмена' }));
    expect(client.updateEvent).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Удалить' }));
    await waitFor(() => expect(client.deleteEvent).toHaveBeenCalledWith('', 't', 'e1'));
  });
});
