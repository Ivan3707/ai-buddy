import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { StaffHomePage } from './StaffHomePage';
import { StaffTeamPage } from './StaffTeamPage';
import { buddyUser, checklist, internUser, item, leadUser } from '../test/fixtures';

vi.mock('../bot/api', () => ({ bot: { questions: vi.fn().mockResolvedValue([]), staffRequests: vi.fn().mockResolvedValue([]) } }));
vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual,
    getAssignedInterns: vi.fn(), getInternProgress: vi.fn(), getPendingSubmissions: vi.fn(),
    reviewSubmission: vi.fn(), listTaskComments: vi.fn(), getInternSubmissionHistory: vi.fn(),
    createPersonalTask: vi.fn(), updateInternTask: vi.fn(), removeInternTask: vi.fn(), adminCompleteChecklists: vi.fn(),
  };
});
const api = vi.mocked(client);

const reviewItem = item({ title: 'Сдать отчёт', description: 'Подробное описание отчёта', expected_result: 'Отчёт в wiki', status: 'completed', requires_review: true });
const prof = checklist({ id: 'p', type: 'professional', stages: [{ id: 's', title: 'Этап 1', week_start: 1, week_end: 1, goal: '', expected_result: '', position: 1, items: [reviewItem] }] });
const progress = { user_id: internUser.id, total_items: 1, completed_items: 1, completion_percent: 100, common: null, professional: prof } as never;
const pending = [{ id: 'sub-1', intern_id: internUser.id, item_id: reviewItem.id, item_title: 'Сдать отчёт', attempt_number: 2, result_text: 'Готово', result_url: null, submitted_at: '2026-11-14T10:00:00Z' }];

beforeEach(() => {
  vi.resetAllMocks();
  api.getAssignedInterns.mockResolvedValue([internUser]);
  api.getInternProgress.mockResolvedValue(progress);
  api.getPendingSubmissions.mockResolvedValue(pending);
  api.listTaskComments.mockResolvedValue([]);
  api.getInternSubmissionHistory.mockResolvedValue([]);
});

describe('Команда наставника', () => {
  it('задача на проверке показывается как «На проверке», а не «Выполнено»', async () => {
    render(<StaffTeamPage user={buddyUser} token="t" />);
    const row = await screen.findByText('Сдать отчёт');
    expect(within(row.closest('.mini-task') as HTMLElement).getByText('На проверке')).toBeInTheDocument();
    expect(screen.queryByText('Выполнено')).not.toBeInTheDocument();
  });

  it('вкладка «На проверку» открывает окно с контекстом задачи', async () => {
    api.reviewSubmission.mockResolvedValue({} as never);
    render(<StaffTeamPage user={buddyUser} token="t" initialTab="review" />);
    await userEvent.click(await screen.findByRole('button', { name: /Сдать отчёт/ }));
    expect(await screen.findByText('Подробное описание отчёта')).toBeInTheDocument();
    expect(screen.getByText(/Отчёт в wiki/)).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByText(/Попытка 2/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Принять' }));
    await waitFor(() => expect(api.reviewSubmission).toHaveBeenCalledWith('', 't', 'sub-1', { approved: true, comment: undefined }));
  });

  it('для возврата нужен комментарий', async () => {
    render(<StaffTeamPage user={buddyUser} token="t" initialTab="review" />);
    await userEvent.click(await screen.findByRole('button', { name: /Сдать отчёт/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Вернуть' }));
    expect((await screen.findAllByRole('alert'))[0]).toHaveTextContent('Для возврата нужен комментарий');
    expect(api.reviewSubmission).not.toHaveBeenCalled();
  });

  it('вкладка переключается вручную', async () => {
    render(<StaffTeamPage user={buddyUser} token="t" />);
    await userEvent.click(await screen.findByRole('tab', { name: /На проверку/ }));
    expect(await screen.findByRole('button', { name: /Сдать отчёт/ })).toBeInTheDocument();
  });
});

describe('Главная наставника', () => {
  it('«Ждут проверки» ведёт на вкладку проверки, кнопки «Все подробности» нет', async () => {
    const onNavigate = vi.fn();
    render(<StaffHomePage user={buddyUser} token="t" onNavigate={onNavigate} />);
    await userEvent.click(await screen.findByRole('button', { name: /Ждут проверки/ }));
    expect(onNavigate).toHaveBeenCalledWith('team', 'review');
    expect(screen.queryByText('Все подробности')).not.toBeInTheDocument();
  });
});

describe('Главная: активные задачи и вопросы', () => {
  it('в карточке стажёра перечислены все активные задачи', async () => {
    const act = (title: string) => item({ title, status: 'active' });
    const list = checklist({ id: 'p2', type: 'professional', stages: [{ id: 's', title: 'Э', week_start: 1, week_end: 1, goal: '', expected_result: '', position: 1, items: [act('Первая активная'), act('Вторая активная'), item({ title: 'Закрытая', status: 'completed' })] }] });
    api.getInternProgress.mockResolvedValue({ ...(progress as object), professional: list } as never);
    render(<StaffHomePage user={buddyUser} token="t" onNavigate={vi.fn()} />);
    expect(await screen.findByText('Активные задачи')).toBeInTheDocument();
    expect(screen.getByText('Первая активная')).toBeInTheDocument();
    expect(screen.getByText('Вторая активная')).toBeInTheDocument();
    expect(screen.queryByText('Закрытая')).not.toBeInTheDocument();
    expect(screen.queryByText('Следующая задача')).not.toBeInTheDocument();
  });

  it('без активных задач пишет «нет активных задач», карточка вопросов переименована', async () => {
    render(<StaffHomePage user={buddyUser} token="t" onNavigate={vi.fn()} />);
    expect(await screen.findByText('нет активных задач')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Вопросы от стажёра/ })).toBeInTheDocument();
  });
});

const commonList = checklist({ id: 'c', type: 'common', stages: [{ id: 's0', title: 'Знакомство', week_start: 1, week_end: 1, goal: '', expected_result: '', position: 1, items: [item({ title: 'Общая задача', status: 'active' })] }], total_items: 1 });
const both = { ...(progress as object), common: commonList, total_items: 2 } as never;

describe('Команда: история ревью', () => {
  it('окно проверки показывает историю всех попыток', async () => {
    api.getInternSubmissionHistory.mockResolvedValue([
      { id: 'h1', attempt_number: 1, status: 'rejected', result_text: 'Первый вариант', result_url: null, submitted_at: '2026-11-13T10:00:00Z', reviewed_at: '2026-11-13T12:00:00Z', review_comment: 'Добавьте скриншоты' },
    ] as never);
    render(<StaffTeamPage user={buddyUser} token="t" initialTab="review" />);
    await userEvent.click(await screen.findByRole('button', { name: /Сдать отчёт/ }));
    expect(await screen.findByText('Добавьте скриншоты')).toBeInTheDocument();
    expect(api.getInternSubmissionHistory).toHaveBeenCalledWith('', 't', internUser.id, reviewItem.id);
  });
});

describe('Команда руководителя', () => {
  beforeEach(() => { api.getInternProgress.mockResolvedValue(both); });

  it('видит оба чек-листа, но не вкладку проверки и не кнопки решения', async () => {
    // Блок «История отправок» в окне задачи появляется только при наличии отправок.
    api.getInternSubmissionHistory.mockResolvedValue([
      { id: 'h0', attempt_number: 1, status: 'pending', result_text: 'Отчёт', result_url: null, submitted_at: '2026-11-13T10:00:00Z', reviewed_at: null, review_comment: null },
    ] as never);
    render(<StaffTeamPage user={leadUser} token="t" />);
    expect(await screen.findByText('Общая задача')).toBeInTheDocument();
    expect(screen.getByText('Сдать отчёт')).toBeInTheDocument();
    expect(screen.queryByRole('tab')).not.toBeInTheDocument();
    expect(api.getPendingSubmissions).not.toHaveBeenCalled();
    await userEvent.click(screen.getByText('Сдать отчёт'));
    expect(screen.queryByRole('button', { name: 'Принять' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Вернуть' })).not.toBeInTheDocument();
    expect(await screen.findByText('История отправок')).toBeInTheDocument();
  });

  it('наставник тоже видит общий чек-лист и не видит правки', async () => {
    render(<StaffTeamPage user={buddyUser} token="t" />);
    expect(await screen.findByText('Общая задача')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Изменить задачу/ })).not.toBeInTheDocument();
  });

  it('добавляет задачу в общий чек-лист', async () => {
    api.createPersonalTask.mockResolvedValue({ id: 'n' });
    render(<StaffTeamPage user={leadUser} token="t" />);
    await userEvent.click(await screen.findByRole('button', { name: '+ Задача' }));
    await userEvent.selectOptions(screen.getByLabelText('Чек-лист'), 'common');
    await userEvent.type(screen.getByLabelText('Название'), 'Новая');
    await userEvent.type(screen.getByLabelText('Ожидаемый результат'), 'Готово');
    await userEvent.click(screen.getByRole('button', { name: 'Создать задачу' }));
    await waitFor(() => expect(api.createPersonalTask).toHaveBeenCalledWith('', 't', internUser.id, expect.objectContaining({ checklist_type: 'common', position: 2, title: 'Новая' })));
  });

  it('редактирует задачу', async () => {
    api.updateInternTask.mockResolvedValue(undefined);
    render(<StaffTeamPage user={leadUser} token="t" />);
    await userEvent.click(await screen.findByRole('button', { name: 'Изменить задачу «Общая задача»' }));
    const title = screen.getByLabelText('Название');
    await userEvent.clear(title);
    await userEvent.type(title, 'Переименована');
    await userEvent.click(screen.getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(api.updateInternTask).toHaveBeenCalledWith('', 't', internUser.id, commonList.stages[0]!.items[0]!.id, expect.objectContaining({ title: 'Переименована' })));
  });

  it('удаляет задачу только после подтверждения; 409 даёт понятное сообщение', async () => {
    api.removeInternTask.mockRejectedValue(new client.ApiError(409, 'conflict'));
    render(<StaffTeamPage user={leadUser} token="t" />);
    await userEvent.click(await screen.findByRole('button', { name: 'Удалить задачу «Общая задача»' }));
    expect(api.removeInternTask).not.toHaveBeenCalled();
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Удалить' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Конфликт');
  });

  it('демо-кнопка завершает чек-листы и обновляет прогресс', async () => {
    api.adminCompleteChecklists.mockResolvedValue({ intern_id: internUser.id, items_completed: 5, submissions_approved: 1 });
    render(<StaffTeamPage user={leadUser} token="t" />);
    await userEvent.click(await screen.findByRole('button', { name: /Завершить чек-листы на 100%/ }));
    expect(await screen.findByText(/завершено задач 5/)).toBeInTheDocument();
    expect(api.adminCompleteChecklists).toHaveBeenCalledWith('', 't', internUser.id);
    expect(api.getInternProgress.mock.calls.length).toBeGreaterThan(1);
  });
});
