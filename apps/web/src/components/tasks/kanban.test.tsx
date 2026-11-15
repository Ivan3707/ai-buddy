import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../../test/fixtures';
import { columnOf } from '../../lib/taskColumns';
import { KanbanBoard } from './KanbanBoard';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual, completeItem: vi.fn(), submitItem: vi.fn(), getSubmissionHistory: vi.fn(),
    listTaskComments: vi.fn(), addTaskComment: vi.fn(), fetchAttachment: vi.fn(),
  };
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
const refreshed = () => vi.fn().mockResolvedValue(undefined);

beforeEach(() => { vi.resetAllMocks(); api.listTaskComments.mockResolvedValue([]); });

describe('columnOf', () => {
  it('«На проверке» — отправленная наставнику, «Завершено» — завершённая без проверки или принятая', () => {
    expect(columnOf({ status: 'completed', requires_review: true })).toBe('review');
    expect(columnOf({ status: 'completed', requires_review: false })).toBe('done');
    expect(columnOf({ status: 'verified', requires_review: true })).toBe('done');
    expect(columnOf({ status: 'active', requires_review: true })).toBe('active');
    expect(columnOf({ status: 'unavailable', requires_review: false })).toBe('unavailable');
  });
});

describe('KanbanBoard', () => {
  it('раскладывает задачи по колонкам: «На проверке» стоит перед «Завершено»', () => {
    const data = checklist();
    data.stages[0].items.push(item({ title: 'Принятая задача', status: 'verified', requires_review: true }));
    data.stages[0].items.push(item({ title: 'Ждёт наставника', status: 'completed', requires_review: true }));
    render(<KanbanBoard checklist={data} token="tok" refresh={vi.fn()} />);
    const titles = screen.getAllByRole('heading', { level: 3 }).map(h => h.textContent);
    expect(titles).toEqual(['Недоступно', 'Активно', 'На проверке', 'Завершено']);
    expect(within(column('Недоступно')).getByText('Третья задача')).toBeInTheDocument();
    expect(within(column('Активно')).getByText('Вторая задача')).toBeInTheDocument();
    expect(within(column('На проверке')).getByText('Ждёт наставника')).toBeInTheDocument();
    expect(within(column('Завершено')).getByText('Первая задача')).toBeInTheDocument();
    expect(within(column('Завершено')).getByText('Принятая задача')).toBeInTheDocument();
  });

  it('у активной карточки есть ручка и кнопка следующего этапа, у остальных нет', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    expect(within(column('На проверке')).getByText('Пусто')).toBeInTheDocument();
    expect(screen.getAllByText('⠿')).toHaveLength(1);
    expect(within(activeCard()).getByRole('button', { name: 'Завершить →' })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Завершить →' })).toHaveLength(1);
  });

  it('кнопка «Завершить» завершает задачу без проверки', async () => {
    const refresh = refreshed();
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refresh} />);
    await userEvent.click(within(activeCard()).getByRole('button', { name: 'Завершить →' }));
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledWith('', 'tok', expect.any(String)));
    expect(api.submitItem).not.toHaveBeenCalled();
    expect(refresh).toHaveBeenCalled();
  });

  it('кнопка «На проверку» не отправляет задачу сразу, а просит комментарий и ссылку в окне задачи', async () => {
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={refreshed()} />);
    await userEvent.click(within(activeCard()).getByRole('button', { name: 'На проверку →' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('добавьте комментарий или ссылку');
    expect(api.submitItem).not.toHaveBeenCalled();
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('перетаскивание в «Завершено» завершает задачу без проверки', async () => {
    const refresh = refreshed();
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refresh} />);
    dragTo(activeCard(), column('Завершено'));
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledWith("", "tok", expect.any(String)));
    await vi.waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it('перетаскивание в «На проверке» задачи без проверки тоже завершает её (сразу «Завершено»)', async () => {
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refreshed()} />);
    dragTo(activeCard(), column('На проверке'));
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledTimes(1));
    expect(api.submitItem).not.toHaveBeenCalled();
  });

  it('перенос задачи с проверкой в «На проверке» или «Завершено» открывает окно с формой, ничего не отправляя', async () => {
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={refreshed()} />);
    dragTo(activeCard(), column('На проверке'));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    dragTo(activeCard(), column('Завершено'));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(api.submitItem).not.toHaveBeenCalled();
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('показывает ошибку, если завершить задачу не удалось (и запасной текст)', async () => {
    api.completeItem.mockRejectedValueOnce(new Error('Нельзя завершить')).mockRejectedValueOnce('boom');
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Завершено'));
    expect(await screen.findByText('Нельзя завершить')).toBeInTheDocument();
    dragTo(activeCard(), column('Завершено'));
    expect(await screen.findByText('Не удалось завершить задачу')).toBeInTheDocument();
  });

  it('назад и в «Недоступно» карточку не вернуть, отпускание мимо ничего не меняет', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Недоступно'));
    expect(screen.getByRole('status')).toHaveTextContent('только вперёд');
    dragTo(activeCard(), null);
    expect(screen.queryByRole('status')).toBeNull();
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('без движения мыши (простой клик) карточка не тянется, а открывает окно задачи', async () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    fireEvent.pointerDown(activeCard(), { pointerId: 1, pointerType: 'mouse', button: 0, clientX: 10, clientY: 10 });
    fireEvent.pointerUp(window, { pointerId: 1, clientX: 10, clientY: 10 });
    await userEvent.click(activeCard());
    expect(api.completeItem).not.toHaveBeenCalled();
    expect(await screen.findByRole('dialog', { name: 'Задача: Вторая задача' })).toBeInTheDocument();
  });

  it('с клавиатуры: Пробел берёт карточку, стрелка вправо переносит, Пробел отпускает; Esc отменяет', async () => {
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refreshed()} />);
    activeCard().focus();
    await userEvent.keyboard(' ');
    expect(screen.getByRole('status')).toHaveTextContent('Карточка взята');
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('status')).toHaveTextContent('Перенос отменён');
    expect(api.completeItem).not.toHaveBeenCalled();
    await userEvent.keyboard(' ');
    await userEvent.keyboard('{ArrowRight}');
    expect(column('На проверке')).toHaveClass('is-over');
    await userEvent.keyboard(' ');
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledTimes(1));
  });

  it('подписывает профессиональный трек и один неделя-этап', () => {
    render(<KanbanBoard checklist={checklistsResponse().professional} token="tok" refresh={vi.fn()} />);
    expect(screen.getByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
    expect(screen.getByText('Недели 3')).toBeInTheDocument();
  });

  it('у недоступных задач нет ручки и кнопки следующего этапа', () => {
    const locked = checklist();
    locked.stages[0]!.items = [item({ status: 'unavailable' })];
    render(<KanbanBoard checklist={locked} token="tok" refresh={vi.fn()} />);
    expect(screen.queryByText('⠿')).toBeNull();
    expect(screen.queryByRole('button', { name: /→/ })).toBeNull();
  });
});

describe('окно задачи', () => {
  const openModal = async (data = checklistsResponse().professional, refresh = refreshed()) => {
    render(<KanbanBoard checklist={data} token="tok" refresh={refresh} />);
    await userEvent.click(within(activeCard()).getByRole('button', { name: /Открыть задачу/ }));
    return screen.findByRole('dialog');
  };

  it('показывает описание, факты и переписку', async () => {
    api.listTaskComments.mockResolvedValue([
      { id: 'c1', author_id: 'u1', author_name: 'Иванов Иван', author_role: 'intern', body: 'Вопрос по задаче', created_at: '2026-11-14T10:00:00Z', attachments: [{ id: 'a1', filename: 'log.txt', content_type: 'text/plain', size: 2048 }] },
      { id: 'c2', author_id: 'u2', author_name: 'Петров Пётр', author_role: 'buddy', body: 'Ответ наставника', created_at: '2026-11-14T11:00:00Z', attachments: [] },
    ]);
    const dialog = await openModal();
    expect(within(dialog).getByText('Описание')).toBeInTheDocument();
    expect(within(dialog).getByText('Проверка')).toBeInTheDocument();
    expect(await within(dialog).findByText('Вопрос по задаче')).toBeInTheDocument();
    expect(within(dialog).getByText('Ответ наставника')).toBeInTheDocument();
    expect(within(dialog).getByText('Наставник')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: /log\.txt/ })).toBeInTheDocument();
    expect(api.listTaskComments).toHaveBeenCalledWith('', 'tok', expect.any(String), undefined);
  });

  it('скачивает вложение', async () => {
    api.listTaskComments.mockResolvedValue([
      { id: 'c1', author_id: 'u1', author_name: 'Иванов Иван', author_role: 'intern', body: '', created_at: '2026-11-14T10:00:00Z', attachments: [{ id: 'a1', filename: 'log.txt', content_type: 'text/plain', size: 10 }] },
    ]);
    api.fetchAttachment.mockResolvedValue(new Blob(['x']));
    URL.createObjectURL = vi.fn(() => 'blob:x');
    URL.revokeObjectURL = vi.fn();
    const dialog = await openModal();
    await userEvent.click(await within(dialog).findByRole('button', { name: /log\.txt/ }));
    await vi.waitFor(() => expect(api.fetchAttachment).toHaveBeenCalledWith('', 'tok', 'a1'));
    expect(URL.createObjectURL).toHaveBeenCalled();
  });

  it('отправляет комментарий с файлом и показывает его в ленте', async () => {
    api.addTaskComment.mockResolvedValue({ id: 'c9', author_id: 'u1', author_name: 'Иванов Иван', author_role: 'intern', body: 'Готово', created_at: '2026-11-14T12:00:00Z', attachments: [] });
    const dialog = await openModal();
    const send = within(dialog).getByRole('button', { name: 'Отправить' });
    expect(send).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText('Новый комментарий'), 'Готово');
    const file = new File(['abc'], 'report.pdf', { type: 'application/pdf' });
    await userEvent.upload(within(dialog).getByLabelText('Выбрать файлы'), file);
    expect(within(dialog).getByText(/report\.pdf/)).toBeInTheDocument();
    await userEvent.click(send);
    await vi.waitFor(() => expect(api.addTaskComment).toHaveBeenCalledWith('', 'tok', expect.any(String), { body: 'Готово', files: [file], internId: undefined }));
    expect(await within(dialog).findByText('Готово')).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Новый комментарий')).toHaveValue('');
  });

  it('не принимает слишком большой файл и показывает ошибку отправки', async () => {
    api.addTaskComment.mockRejectedValue(new Error('Сервер отказал'));
    const dialog = await openModal();
    const big = new File(['x'], 'big.zip');
    Object.defineProperty(big, 'size', { value: 6 * 1024 * 1024 });
    await userEvent.upload(within(dialog).getByLabelText('Выбрать файлы'), big);
    expect(within(dialog).getByRole('alert')).toHaveTextContent('больше 5 МБ');
    await userEvent.type(within(dialog).getByLabelText('Новый комментарий'), 'Привет');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Отправить' }));
    expect(await within(dialog).findByText('Сервер отказал')).toBeInTheDocument();
  });

  it('показывает ошибку загрузки комментариев и повторяет запрос', async () => {
    api.listTaskComments.mockRejectedValueOnce(new Error('Нет связи')).mockResolvedValueOnce([]);
    const dialog = await openModal();
    expect(await within(dialog).findByText(/Нет связи/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Повторить' }));
    expect(await within(dialog).findByText(/Пока нет сообщений/)).toBeInTheDocument();
  });

  it('отправка на проверку недоступна без комментария или ссылки; с ними задача уходит наставнику', async () => {
    const refresh = refreshed();
    api.submitItem.mockResolvedValue({} as never);
    const dialog = await openModal(undefined, refresh);
    const send = within(dialog).getByRole('button', { name: 'На проверку →' });
    expect(send).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText('Комментарий к результату'), 'Сделал');
    await userEvent.type(within(dialog).getByLabelText('Ссылка на результат'), 'https://example.com/pr/1');
    await userEvent.click(send);
    await vi.waitFor(() => expect(api.submitItem).toHaveBeenCalledWith('', 'tok', expect.any(String), { result_text: 'Сделал', result_url: 'https://example.com/pr/1' }));
    expect(refresh).toHaveBeenCalled();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.getByRole('status')).toHaveTextContent('отправлена на проверку');
  });

  it('достаточно одной ссылки; ошибка отправки видна в окне', async () => {
    api.submitItem.mockRejectedValueOnce(new Error('Отказ сервера')).mockResolvedValueOnce({} as never);
    const dialog = await openModal();
    await userEvent.type(within(dialog).getByLabelText('Ссылка на результат'), 'https://example.com/x');
    await userEvent.click(within(dialog).getByRole('button', { name: 'На проверку →' }));
    expect(await within(dialog).findByText('Отказ сервера')).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole('button', { name: 'На проверку →' }));
    await vi.waitFor(() => expect(api.submitItem).toHaveBeenLastCalledWith('', 'tok', expect.any(String), { result_text: null, result_url: 'https://example.com/x' }));
  });

  it('переносит задачу без проверки кнопкой «Завершить»', async () => {
    api.completeItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={refreshed()} />);
    await userEvent.click(within(activeCard()).getByRole('button', { name: /Открыть задачу/ }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Завершить →' }));
    await vi.waitFor(() => expect(api.completeItem).toHaveBeenCalledTimes(1));
  });

  it('закрывается по Esc и по крестику', async () => {
    await openModal();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    await userEvent.click(within(activeCard()).getByRole('button', { name: /Открыть задачу/ }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Закрыть' }));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('раскрывает историю отправок с комментарием наставника', async () => {
    api.getSubmissionHistory.mockResolvedValue([
      { id: 's1', progress_id: 'p', attempt_number: 1, result_text: 'Текст', result_url: 'https://example.com/x', status: 'rejected', submitted_at: '2026-11-14T10:00:00Z', reviewed_at: '2026-11-14T11:00:00Z', review_comment: 'Нужны правки' },
      { id: 's2', progress_id: 'p', attempt_number: 2, result_text: null, result_url: null, status: 'approved', submitted_at: '2026-11-14T12:00:00Z', reviewed_at: null, review_comment: null },
      { id: 's3', progress_id: 'p', attempt_number: 3, result_text: null, result_url: null, status: 'pending', submitted_at: '2026-11-14T13:00:00Z', reviewed_at: null, review_comment: null },
    ]);
    const dialog = await openModal();
    await userEvent.click(within(dialog).getByRole('button', { name: 'История отправок' }));
    expect(await within(dialog).findByText('Нужны правки')).toBeInTheDocument();
    expect(within(dialog).getByText('Отклонено')).toBeInTheDocument();
    expect(within(dialog).getByText('Одобрено')).toBeInTheDocument();
    expect(within(dialog).getByText('На проверке')).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Скрыть историю' }));
    expect(within(dialog).queryByText('Нужны правки')).toBeNull();
  });

  it('сообщает, когда отправок не было и когда история не загрузилась (с сообщением и без)', async () => {
    api.getSubmissionHistory.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('Нет истории')).mockRejectedValueOnce('boom');
    const dialog = await openModal();
    const open = () => userEvent.click(within(dialog).getByRole('button', { name: 'История отправок' }));
    const close = () => userEvent.click(within(dialog).getByRole('button', { name: 'Скрыть историю' }));
    await open();
    expect(await within(dialog).findByText('Отправок пока нет.')).toBeInTheDocument();
    await close(); await open();
    expect(await within(dialog).findByText('Нет истории')).toBeInTheDocument();
    await close(); await open();
    expect(await within(dialog).findByText('Не удалось загрузить историю')).toBeInTheDocument();
  });
});
