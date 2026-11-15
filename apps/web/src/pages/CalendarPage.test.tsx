import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CalendarPage } from './CalendarPage';

const client = vi.hoisted(() => ({
  listEvents: vi.fn(async () => []),
  createEvent: vi.fn(async () => ({})),
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
});
