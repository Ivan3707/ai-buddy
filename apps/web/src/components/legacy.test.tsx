import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../test/fixtures';
import { BuddyDashboard } from './BuddyDashboard';
import { ChecklistView } from './ChecklistView';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual,
    completeItem: vi.fn(),
    submitItem: vi.fn(),
    getSubmissionHistory: vi.fn(),
    createIntern: vi.fn(),
    getPendingSubmissions: vi.fn(),
    reviewSubmission: vi.fn(),
    chat: vi.fn(),
  };
});

const api = vi.mocked(client);
const pending = {
  id: 'sub-1', intern_id: 'intern-1', item_id: 'item-9', item_title: 'Согласовать цели', attempt_number: 2,
  result_text: 'Мой результат', result_url: 'https://example.com/result', submitted_at: '2026-11-14T10:00:00Z',
};

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

describe('BuddyDashboard', () => {
  it('показывает пустую очередь', async () => {
    api.getPendingSubmissions.mockResolvedValue([]);
    render(<BuddyDashboard token="tok" />);
    expect(await screen.findByText('Всё проверено')).toBeInTheDocument();
  });

  it('показывает ошибку загрузки очереди', async () => {
    api.getPendingSubmissions.mockRejectedValue(new Error('Очередь недоступна'));
    render(<BuddyDashboard token="tok" />);
    expect(await screen.findByText('Очередь недоступна')).toBeInTheDocument();
  });

  it('использует запасной текст, если очередь не загрузилась без сообщения', async () => {
    api.getPendingSubmissions.mockRejectedValue('boom');
    render(<BuddyDashboard token="tok" />);
    expect(await screen.findByText('Не удалось загрузить очередь')).toBeInTheDocument();
  });

  it('одобряет работу стажёра и обновляет очередь', async () => {
    api.getPendingSubmissions.mockResolvedValueOnce([pending]).mockResolvedValue([]);
    api.reviewSubmission.mockResolvedValue(undefined);
    render(<BuddyDashboard token="tok" />);
    expect(await screen.findByText('Согласовать цели')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Открыть приложенный результат/ })).toHaveAttribute('href', 'https://example.com/result');
    await userEvent.click(screen.getByRole('button', { name: 'Одобрить' }));
    expect(api.reviewSubmission).toHaveBeenCalledWith('', 'tok', 'sub-1', { approved: true });
    expect(await screen.findByText('Всё проверено')).toBeInTheDocument();
  });

  it('требует комментарий при отклонении', async () => {
    api.getPendingSubmissions.mockResolvedValue([pending]);
    api.reviewSubmission.mockResolvedValue(undefined);
    render(<BuddyDashboard token="tok" />);
    await screen.findByText('Согласовать цели');
    await userEvent.click(screen.getByRole('button', { name: 'Отклонить' }));
    expect(screen.getByText('Для отклонения нужен комментарий.')).toBeInTheDocument();
    expect(api.reviewSubmission).not.toHaveBeenCalled();
    await userEvent.type(screen.getByLabelText('Комментарий при отклонении'), 'Доработайте');
    await userEvent.click(screen.getByRole('button', { name: 'Отклонить' }));
    expect(api.reviewSubmission).toHaveBeenCalledWith('', 'tok', 'sub-1', { approved: false, comment: 'Доработайте' });
  });

  it('показывает ошибку проверки', async () => {
    api.getPendingSubmissions.mockResolvedValue([pending]);
    api.reviewSubmission.mockRejectedValue(new Error('Не сохранилось'));
    render(<BuddyDashboard token="tok" />);
    await screen.findByText('Согласовать цели');
    await userEvent.click(screen.getByRole('button', { name: 'Одобрить' }));
    expect(await screen.findByText('Не сохранилось')).toBeInTheDocument();
  });

  it('использует запасной текст, если решение не сохранилось без сообщения', async () => {
    api.getPendingSubmissions.mockResolvedValue([pending]);
    api.reviewSubmission.mockRejectedValue('boom');
    render(<BuddyDashboard token="tok" />);
    await screen.findByText('Согласовать цели');
    await userEvent.click(screen.getByRole('button', { name: 'Одобрить' }));
    expect(await screen.findByText('Не удалось сохранить решение')).toBeInTheDocument();
  });

  it('создаёт стажёра', async () => {
    api.getPendingSubmissions.mockResolvedValue([]);
    api.createIntern.mockResolvedValue({ id: 'u', email: 'new@example.com', role: 'intern', profession: 'backend', created_at: '2026-11-14T10:00:00Z' });
    render(<BuddyDashboard token="tok" />);
    const form = await screen.findByRole('region', { name: 'Создать стажёра' });
    await userEvent.type(within(form).getByLabelText('Email'), 'new@example.com');
    await userEvent.type(within(form).getByLabelText('Временный пароль'), 'long-enough-password');
    await userEvent.click(within(form).getByRole('button', { name: 'Создать стажёра' }));
    expect(await within(form).findByText(/Стажёр new@example.com создан/)).toBeInTheDocument();
    expect(api.createIntern).toHaveBeenCalledWith('', 'tok', { email: 'new@example.com', password: 'long-enough-password', profession: 'backend' });
  });

  it('показывает ошибку создания стажёра', async () => {
    api.getPendingSubmissions.mockResolvedValue([]);
    api.createIntern.mockRejectedValue(new Error('Email занят'));
    render(<BuddyDashboard token="tok" />);
    const form = await screen.findByRole('region', { name: 'Создать стажёра' });
    await userEvent.type(within(form).getByLabelText('Email'), 'dup@example.com');
    await userEvent.type(within(form).getByLabelText('Временный пароль'), 'long-enough-password');
    await userEvent.click(within(form).getByRole('button', { name: 'Создать стажёра' }));
    expect(await within(form).findByText('Email занят')).toBeInTheDocument();
  });

  it('использует запасной текст, если стажёр не создался без сообщения', async () => {
    api.getPendingSubmissions.mockResolvedValue([]);
    api.createIntern.mockRejectedValue('boom');
    render(<BuddyDashboard token="tok" />);
    const form = await screen.findByRole('region', { name: 'Создать стажёра' });
    await userEvent.type(within(form).getByLabelText('Email'), 'x@example.com');
    await userEvent.type(within(form).getByLabelText('Временный пароль'), 'long-enough-password');
    await userEvent.click(within(form).getByRole('button', { name: 'Создать стажёра' }));
    expect(await within(form).findByText('Не удалось создать стажёра')).toBeInTheDocument();
  });

  it('обновляет очередь по кнопке', async () => {
    api.getPendingSubmissions.mockResolvedValue([]);
    render(<BuddyDashboard token="tok" />);
    await screen.findByText('Всё проверено');
    await userEvent.click(screen.getByRole('button', { name: /Обновить/ }));
    expect(api.getPendingSubmissions).toHaveBeenCalledTimes(2);
  });
});
