import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MentorDesk } from './MentorDesk';

const q = { id: 'q1', intern_name: 'Спайк', question: 'Как получить доступ?', context: '', status: 'open', answer: null, created_at: '2026-11-14T10:00:00Z' };
vi.mock('./api', () => ({ bot: { metrics: vi.fn(), questions: vi.fn(), draft: vi.fn(), answer: vi.fn() } }));
import { bot } from './api';
const api = vi.mocked(bot);

beforeEach(() => {
  vi.resetAllMocks();
  api.metrics.mockResolvedValue(null as never);
  api.questions.mockResolvedValue([q] as never);
  api.answer.mockResolvedValue(undefined as never);
});

describe('вопросы стажёров', () => {
  it('наставник отвечает сам кнопкой «Ответить»', async () => {
    render(<MentorDesk token="t" />);
    expect(await screen.findByRole('button', { name: /AI-черновик ответа/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Ответить' }));
    await userEvent.type(screen.getByLabelText('Ваш ответ'), 'Напиши в чат команды');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить стажёру' }));
    await waitFor(() => expect(api.answer).toHaveBeenCalledWith('t', 'q1', 'Напиши в чат команды'));
  });
});
