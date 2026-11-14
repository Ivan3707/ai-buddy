import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../test/fixtures';
import { ChecklistView } from './ChecklistView';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual,
    completeItem: vi.fn(),
    submitItem: vi.fn(),
    getSubmissionHistory: vi.fn(),
  };
});

const api = vi.mocked(client);

beforeEach(() => {
  vi.resetAllMocks();
});

describe('ChecklistView', () => {
  it('показывает прогресс и завершает активную задачу', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    api.completeItem.mockResolvedValue(undefined);
    render(<ChecklistView checklist={checklist()} token="tok" refresh={refresh} />);
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '33');
    expect(screen.getByText('1 из 3 задач завершено')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Завершить' }));
    expect(api.completeItem).toHaveBeenCalledWith('', 'tok', expect.any(String));
    expect(refresh).toHaveBeenCalled();
  });

  it('показывает ошибку, если завершить задачу не удалось', async () => {
    api.completeItem.mockRejectedValue(new Error('Нельзя завершить'));
    render(<ChecklistView checklist={checklist()} token="tok" refresh={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'Завершить' }));
    expect(await screen.findByText('Нельзя завершить')).toBeInTheDocument();
  });

  it('использует запасной текст при неизвестной ошибке', async () => {
    api.completeItem.mockRejectedValue('boom');
    render(<ChecklistView checklist={checklist()} token="tok" refresh={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'Завершить' }));
    expect(await screen.findByText('Не удалось завершить задачу')).toBeInTheDocument();
  });

  it('подписывает профессиональный трек и один неделя-этап', () => {
    const { professional } = checklistsResponse();
    render(<ChecklistView checklist={professional} token="tok" refresh={vi.fn()} />);
    expect(screen.getByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
    expect(screen.getByText('Недели 3')).toBeInTheDocument();
  });

  it('отправляет результат задачи с проверкой', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    api.submitItem.mockResolvedValue({} as never);
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={refresh} />);
    const send = screen.getByRole('button', { name: 'Отправить на проверку' });
    expect(send).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Что сделано'), 'Сделал');
    await userEvent.type(screen.getByLabelText('Ссылка на результат'), 'https://example.com/pr/1');
    await userEvent.click(send);
    expect(api.submitItem).toHaveBeenCalledWith('', 'tok', expect.any(String), { result_text: 'Сделал', result_url: 'https://example.com/pr/1' });
    expect(refresh).toHaveBeenCalled();
  });

  it('показывает ошибку отправки результата', async () => {
    api.submitItem.mockRejectedValue(new Error('Отказ сервера'));
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    await userEvent.type(screen.getByLabelText('Что сделано'), 'Сделал');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить на проверку' }));
    expect(await screen.findByText('Отказ сервера')).toBeInTheDocument();
  });

  it('показывает запасной текст при неизвестной ошибке отправки', async () => {
    api.submitItem.mockRejectedValue('boom');
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    await userEvent.type(screen.getByLabelText('Что сделано'), 'Сделал');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить на проверку' }));
    expect(await screen.findByText('Не удалось отправить результат')).toBeInTheDocument();
  });

  it('раскрывает историю отправок с комментарием buddy', async () => {
    api.getSubmissionHistory.mockResolvedValue([
      { id: 's1', progress_id: 'p', attempt_number: 1, result_text: 'Текст', result_url: 'https://example.com/x', status: 'rejected', submitted_at: '2026-11-14T10:00:00Z', reviewed_at: '2026-11-14T11:00:00Z', review_comment: 'Нужны правки' },
      { id: 's2', progress_id: 'p', attempt_number: 2, result_text: null, result_url: null, status: 'approved', submitted_at: '2026-11-14T12:00:00Z', reviewed_at: null, review_comment: null },
      { id: 's3', progress_id: 'p', attempt_number: 3, result_text: null, result_url: null, status: 'pending', submitted_at: '2026-11-14T13:00:00Z', reviewed_at: null, review_comment: null },
    ]);
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    expect(await screen.findByText('Нужны правки')).toBeInTheDocument();
    expect(screen.getByText('Отклонено')).toBeInTheDocument();
    expect(screen.getByText('Одобрено')).toBeInTheDocument();
    expect(screen.getByText('На проверке')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Скрыть историю' }));
    expect(screen.queryByText('Нужны правки')).toBeNull();
  });

  it('сообщает, когда отправок ещё не было, и когда история не загрузилась', async () => {
    api.getSubmissionHistory.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('Нет истории'));
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    expect(await screen.findByText('Отправок пока нет.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Скрыть историю' }));
    await userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    expect(await screen.findByText('Нет истории')).toBeInTheDocument();
  });

  it('использует запасной текст, если история не загрузилась без сообщения', async () => {
    api.getSubmissionHistory.mockRejectedValue('boom');
    render(<ChecklistView checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    expect(await screen.findByText('Не удалось загрузить историю')).toBeInTheDocument();
  });

  it('не показывает кнопок у недоступных задач', () => {
    const locked = checklist();
    locked.stages[0]!.items = [item({ status: 'unavailable' })];
    render(<ChecklistView checklist={locked} token="tok" refresh={vi.fn()} />);
    expect(screen.queryByRole('button', { name: 'Завершить' })).toBeNull();
  });
});
