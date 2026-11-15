import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { RequestsDesk } from './RequestsDesk';

const req = (id: string, status: string) => ({
  request: { id, number: `ABS-${id}`, type: 'leave_request', status, params: { kind: 'day_off', date_from: '2026-12-01', date_to: '2026-12-01' } },
  intern_id: 'i', intern_name: 'Стажёр',
});
vi.mock('./api', async orig => ({
  ...(await orig<typeof import('./api')>()),
  bot: { staffRequests: vi.fn(async () => [req('1', 'pending_approval'), req('2', 'approved'), req('3', 'rejected')]) },
}));

describe('RequestsDesk', () => {
  it('кнопка и статусы используют один класс формы', async () => {
    render(<RequestsDesk token="t" />);
    expect(await screen.findByText('Согласовать')).toHaveClass('req-action', 'is-action');
    expect(screen.getByText('согласована')).toHaveClass('req-action', 'is-approved');
    expect(screen.getByText('отклонена')).toHaveClass('req-action', 'is-rejected');
  });
});
