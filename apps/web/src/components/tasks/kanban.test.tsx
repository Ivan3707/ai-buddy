import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../../test/fixtures';
import { KanbanBoard } from './KanbanBoard';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return { ...actual, completeItem: vi.fn(), submitItem: vi.fn(), getSubmissionHistory: vi.fn() };
});

const api = vi.mocked(client);
const column = (name: string) => screen.getByRole('region', { name });

/** Тянет карточку за ручку и отпускает над колонкой (jsdom не считает раскладку, поэтому «что под курсором» подставляем). */
function dragTo(card: HTMLElement, target: HTMLElement | null) {
  const handle = within(card).getByText('⠿');
  fireEvent.pointerDown(handle, { pointerId: 1, pointerType: 'mouse', button: 0, clientX: 10, clientY: 10 });
  document.elementsFromPoint = () => (target ? [target] : []);
  fireEvent.pointerMove(window, { pointerId: 1, clientX: 220, clientY: 140 });
  fireEvent.pointerUp(window, { pointerId: 1, clientX: 220, clientY: 140 });
}
const activeCard = () => within(column('Активно')).getAllByRole('article')[0]!;

beforeEach(() => { vi.resetAllMocks(); });

describe('KanbanBoard', () => {
  it('раскладывает задачи по колонкам статусов', () => {
    const data = checklist();
    data.stages[0].items.push(item({ title: 'Проверенная задача', status: 'verified' }));
    render(<KanbanBoard checklist={data} token="tok" refresh={vi.fn()} />);
    const titles = screen.getAllByRole('heading', { level: 3 }).map(h => h.textContent);
    expect(titles).toEqual(['Недоступно', 'Активно', 'Завершено', 'Проверено']);
    expect(within(column('Недоступно')).getByText('Третья задача')).toBeInTheDocument();
    expect(within(column('Активно')).getByText('Вторая задача')).toBeInTheDocument();
    expect(within(column('Завершено')).getByText('Первая задача')).toBeInTheDocument();
    expect(within(column('Проверено')).getByText('Проверенная задача')).toBeInTheDocument();
  });

  it('кнопки «Завершить» нет, пустая колонка помечена, а ручка есть только у активной карточки', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    expect(screen.queryByRole('button', { name: 'Завершить' })).toBeNull();
    expect(within(column('Проверено')).getByText('Пусто')).toBeInTheDocument();
    expect(screen.getAllByText('⠿')).toHaveLength(1);
    expect(within(activeCard()).getByText('⠿')).toBeInTheDocument();
  });

  it('перетаскивание активной карточки в «Завершено» завершает задачу', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refresh} />);
    dragTo(activeCard(), column('Завершено'));
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledWith('', 'tok', expect.any(String)));
    expect(refresh).toHaveBeenCalled();
  });

  it('показывает ошибку, если завершить задачу не удалось (и запасной текст)', async () => {
    api.completeItem.mockRejectedValueOnce(new Error('Нельзя завершить')).mockRejectedValueOnce('boom');
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Завершено'));
    expect(await screen.findByText('Нельзя завершить')).toBeInTheDocument();
    dragTo(activeCard(), column('Завершено'));
    expect(await screen.findByText('Не удалось завершить задачу')).toBeInTheDocument();
  });

  it('отпускание над другой колонкой или мимо ничего не меняет, «Проверено» подсказывает про наставника', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Проверено'));
    expect(screen.getByRole('status')).toHaveTextContent('только в «Завершено»');
    dragTo(activeCard(), null);
    expect(screen.queryByRole('status')).toBeNull();
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('без движения мыши (простой клик) карточка не тянется', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    const handle = within(activeCard()).getByText('⠿');
    fireEvent.pointerDown(handle, { pointerId: 1, pointerType: 'mouse', button: 0, clientX: 10, clientY: 10 });
    fireEvent.pointerUp(window, { pointerId: 1, clientX: 10, clientY: 10 });
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('с клавиатуры: Пробел берёт карточку, стрелка вправо переносит, Пробел отпускает; Esc отменяет', async () => {
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn().mockResolvedValue(undefined)} />);
    activeCard().focus();
    await userEvent.keyboard(' ');
    expect(screen.getByRole('status')).toHaveTextContent('Карточка взята');
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('status')).toHaveTextContent('Перенос отменён');
    expect(api.completeItem).not.toHaveBeenCalled();
    await userEvent.keyboard(' ');
    await userEvent.keyboard('{ArrowRight}');
    expect(column('Завершено')).toHaveClass('is-over');
    await userEvent.keyboard(' ');
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledTimes(1));
  });

  it('задача с проверкой не завершается переносом: подсказка про форму результата', () => {
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Завершено'));
    expect(screen.getByRole('status')).toHaveTextContent('принимает наставник');
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('подписывает профессиональный трек и один неделя-этап', () => {
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    expect(screen.getByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
    expect(screen.getByText('Недели 3')).toBeInTheDocument();
  });

  it('у недоступных задач нет ручки', () => {
    const locked = checklist();
    locked.stages[0]!.items = [item({ status: 'unavailable' })];
    render(<KanbanBoard checklist={locked} token="tok" refresh={vi.fn()} />);
    expect(screen.queryByText('⠿')).toBeNull();
  });
});

describe('отправка результата и история (карточка с проверкой)', () => {
  const board = (refresh = vi.fn().mockResolvedValue(undefined)) =>
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={refresh} />);

  it('отправляет результат задачи с проверкой', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    api.submitItem.mockResolvedValue({} as never);
    board(refresh);
    const send = screen.getByRole('button', { name: 'Отправить на проверку' });
    expect(send).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Что сделано'), 'Сделал');
    await userEvent.type(screen.getByLabelText('Ссылка на результат'), 'https://example.com/pr/1');
    await userEvent.click(send);
    expect(api.submitItem).toHaveBeenCalledWith('', 'tok', expect.any(String), { result_text: 'Сделал', result_url: 'https://example.com/pr/1' });
    expect(refresh).toHaveBeenCalled();
  });

  it('показывает ошибку отправки результата и запасной текст', async () => {
    api.submitItem.mockRejectedValueOnce(new Error('Отказ сервера')).mockRejectedValueOnce('boom');
    board();
    await userEvent.type(screen.getByLabelText('Что сделано'), 'Сделал');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить на проверку' }));
    expect(await screen.findByText('Отказ сервера')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Отправить на проверку' }));
    expect(await screen.findByText('Не удалось отправить результат')).toBeInTheDocument();
  });

  it('раскрывает историю отправок с комментарием наставника', async () => {
    api.getSubmissionHistory.mockResolvedValue([
      { id: 's1', progress_id: 'p', attempt_number: 1, result_text: 'Текст', result_url: 'https://example.com/x', status: 'rejected', submitted_at: '2026-11-14T10:00:00Z', reviewed_at: '2026-11-14T11:00:00Z', review_comment: 'Нужны правки' },
      { id: 's2', progress_id: 'p', attempt_number: 2, result_text: null, result_url: null, status: 'approved', submitted_at: '2026-11-14T12:00:00Z', reviewed_at: null, review_comment: null },
      { id: 's3', progress_id: 'p', attempt_number: 3, result_text: null, result_url: null, status: 'pending', submitted_at: '2026-11-14T13:00:00Z', reviewed_at: null, review_comment: null },
    ]);
    board();
    await userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    expect(await screen.findByText('Нужны правки')).toBeInTheDocument();
    expect(screen.getByText('Отклонено')).toBeInTheDocument();
    expect(screen.getByText('Одобрено')).toBeInTheDocument();
    expect(screen.getByText('На проверке')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Скрыть историю' }));
    expect(screen.queryByText('Нужны правки')).toBeNull();
  });

  it('сообщает, когда отправок не было и когда история не загрузилась (с сообщением и без)', async () => {
    api.getSubmissionHistory.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('Нет истории')).mockRejectedValueOnce('boom');
    board();
    const open = () => userEvent.click(screen.getByRole('button', { name: 'История отправок' }));
    const close = () => userEvent.click(screen.getByRole('button', { name: 'Скрыть историю' }));
    await open();
    expect(await screen.findByText('Отправок пока нет.')).toBeInTheDocument();
    await close(); await open();
    expect(await screen.findByText('Нет истории')).toBeInTheDocument();
    await close(); await open();
    expect(await screen.findByText('Не удалось загрузить историю')).toBeInTheDocument();
  });
});
