import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../../test/fixtures';
import { columnOf, isAvailable, statusLabel } from '../../lib/taskColumns';
import { KanbanBoard } from './KanbanBoard';
import { TaskModal } from './TaskModal';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual, completeItem: vi.fn(), submitItem: vi.fn(), startItem: vi.fn(), getSubmissionHistory: vi.fn(),
    getInternSubmissionHistory: vi.fn(), listTaskComments: vi.fn(), addTaskComment: vi.fn(), fetchAttachment: vi.fn(),
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

  it('открытая, но не начатая задача стоит в «Задачи»; без поля started задача считается начатой', () => {
    expect(columnOf({ status: 'active', requires_review: false, started: false })).toBe('unavailable');
    expect(columnOf({ status: 'active', requires_review: false, started: true })).toBe('active');
    expect(isAvailable({ status: 'active', started: false })).toBe(true);
    expect(isAvailable({ status: 'active' })).toBe(false);
    expect(isAvailable({ status: 'unavailable', started: false })).toBe(false);
    expect(statusLabel({ status: 'unavailable', requires_review: false })).toBe('Недоступно');
    expect(statusLabel({ status: 'active', requires_review: false, started: false })).toBe('Можно начать');
    expect(statusLabel({ status: 'completed', requires_review: true })).toBe('На проверке');
  });
});

describe('KanbanBoard', () => {
  it('раскладывает задачи по колонкам: «На проверке» стоит перед «Завершено»', () => {
    const data = checklist();
    data.stages[0].items.push(item({ title: 'Принятая задача', status: 'verified', requires_review: true }));
    data.stages[0].items.push(item({ title: 'Ждёт наставника', status: 'completed', requires_review: true }));
    render(<KanbanBoard checklist={data} token="tok" refresh={vi.fn()} />);
    const titles = screen.getAllByRole('heading', { level: 3 }).map(h => h.textContent);
    expect(titles).toEqual(['Задачи', 'Активно', 'На проверке', 'Завершено']);
    expect(within(column('Задачи')).getByText('Третья задача')).toBeInTheDocument();
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

  it('назад в «Задачи» карточку не вернуть, отпускание мимо ничего не меняет', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    dragTo(activeCard(), column('Задачи'));
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

  it('при наведении в «Активно» замок не появляется: в кружке активной карточки нет значка', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    const circle = within(activeCard()).getByRole('button', { name: /Открыть задачу/ });
    expect(circle.querySelector('svg')).toBeNull();
    expect(document.querySelector('.kanban-hover-lock')).toBeNull();
  });
});

describe('Колонка «Задачи»: стажёр сам берёт задачу в работу', () => {
  /** Как после завершения задачи: следующая открыта (active, started=false), дальше заблокированные. */
  const withAvailable = () => {
    const data = checklist();
    data.stages[0]!.items = [
      item({ title: 'Сделанная', status: 'completed' }),
      item({ title: 'Заблокированная', status: 'unavailable' }),
      item({ title: 'Можно начать', status: 'active', started: false }),
    ];
    return data;
  };
  const todo = () => within(column('Задачи')).getAllByRole('article');

  it('доступная задача стоит первой, ниже заблокированные; кнопка «В работу» только у доступной', () => {
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={vi.fn()} />);
    expect(todo().map(card => within(card).getByRole('heading', { level: 4 }).textContent)).toEqual(['Можно начать', 'Заблокированная']);
    expect(within(todo()[0]!).getByRole('button', { name: 'В работу →' })).toBeInTheDocument();
    expect(within(todo()[1]!).queryByRole('button', { name: /→/ })).toBeNull();
    expect(within(column('Активно')).getByText('Пусто')).toBeInTheDocument();
  });

  it('кнопка «В работу» переводит задачу в «Активно» через бэкенд', async () => {
    const refresh = refreshed();
    api.startItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={refresh} />);
    await userEvent.click(within(todo()[0]!).getByRole('button', { name: 'В работу →' }));
    await vi.waitFor(() => expect(api.startItem).toHaveBeenCalledWith('', 'tok', expect.any(String)));
    expect(refresh).toHaveBeenCalled();
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('перетаскивание в «Активно» берёт задачу в работу; сразу в «Завершено» нельзя', async () => {
    api.startItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={refreshed()} />);
    dragTo(todo()[0]!, column('Завершено'));
    expect(screen.getByRole('status')).toHaveTextContent('перенесите её в «Активно»');
    expect(api.startItem).not.toHaveBeenCalled();
    dragTo(todo()[0]!, column('Активно'));
    await vi.waitFor(() => expect(api.startItem).toHaveBeenCalledTimes(1));
    expect(api.completeItem).not.toHaveBeenCalled();
  });

  it('с клавиатуры стрелка вправо подсвечивает «Активно»', async () => {
    api.startItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={refreshed()} />);
    todo()[0]!.focus();
    await userEvent.keyboard(' ');
    await userEvent.keyboard('{ArrowRight}');
    expect(column('Активно')).toHaveClass('is-over');
    await userEvent.keyboard(' ');
    await vi.waitFor(() => expect(api.startItem).toHaveBeenCalledTimes(1));
  });

  it('ошибка старта видна на доске (и запасной текст)', async () => {
    api.startItem.mockRejectedValueOnce(new Error('Задача заблокирована')).mockRejectedValueOnce('boom');
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={vi.fn()} />);
    await userEvent.click(within(todo()[0]!).getByRole('button', { name: 'В работу →' }));
    expect(await screen.findByText('Задача заблокирована')).toBeInTheDocument();
    await userEvent.click(within(todo()[0]!).getByRole('button', { name: 'В работу →' }));
    expect(await screen.findByText('Не удалось взять задачу в работу')).toBeInTheDocument();
  });

  it('в окне доступной задачи — «В работу», а не следующий этап', async () => {
    api.startItem.mockResolvedValue(undefined);
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={refreshed()} />);
    await userEvent.click(within(todo()[0]!).getByRole('button', { name: /Открыть задачу/ }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Можно начать', { selector: 'dd' })).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Завершить →' })).toBeNull();
    await userEvent.click(within(dialog).getByRole('button', { name: 'В работу →' }));
    await vi.waitFor(() => expect(api.startItem).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('ошибка старта из окна видна в окне', async () => {
    api.startItem.mockRejectedValue(new Error('Нет связи'));
    render(<KanbanBoard checklist={withAvailable()} token="tok" refresh={refreshed()} />);
    await userEvent.click(within(todo()[0]!).getByRole('button', { name: /Открыть задачу/ }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.click(within(dialog).getByRole('button', { name: 'В работу →' }));
    expect(await within(dialog).findByText('Нет связи')).toBeInTheDocument();
  });
});

describe('окно задачи', () => {
  const openModal = async (data = checklistsResponse().professional, refresh = refreshed()) => {
    render(<KanbanBoard checklist={data} token="tok" refresh={refresh} />);
    await userEvent.click(within(activeCard()).getByRole('button', { name: /Открыть задачу/ }));
    return screen.findByRole('dialog');
  };

  const openReviewModal = async () => {
    const data = checklist();
    data.stages[0]!.items = [item({ title: 'Ждёт наставника', status: 'completed', requires_review: true })];
    render(<KanbanBoard checklist={data} token="tok" refresh={refreshed()} />);
    await userEvent.click(within(within(column('На проверке')).getAllByRole('article')[0]!).getByRole('button', { name: /Открыть задачу/ }));
    return screen.findByRole('dialog');
  };

  it('комментарии и файлы открыты на любом этапе: в работе, на проверке, завершённая и недоступная', async () => {
    const statuses = ['active', 'completed', 'verified', 'unavailable'] as const;
    for (const status of statuses) {
      const { unmount } = render(<TaskModal item={item({ status, requires_review: true })} token="tok" onClose={vi.fn()} />);
      const dialog = screen.getByRole('dialog');
      expect(within(dialog).getByLabelText('Новый комментарий')).toBeInTheDocument();
      expect(within(dialog).getByRole('button', { name: /Прикрепить файл/ })).toBeInTheDocument();
      expect(within(dialog).getByRole('button', { name: 'Отправить' })).toBeInTheDocument();
      unmount();
    }
    const dialog = await openModal();
    expect(within(dialog).getByLabelText('Новый комментарий')).toBeInTheDocument();
  });

  it('наставник и руководитель тоже пишут комментарии: сообщение уходит с id стажёра', async () => {
    api.addTaskComment.mockResolvedValue({ id: 'c9', author_id: 'b', author_name: 'Наставник', author_role: 'buddy', body: 'Ок', created_at: '2026-11-14T12:00:00Z', attachments: [] });
    render(<TaskModal item={item({ status: 'active', requires_review: true })} token="tok" internId="intern-1" onClose={vi.fn()} />);
    await userEvent.type(screen.getByLabelText('Новый комментарий'), 'Ок');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
    await vi.waitFor(() => expect(api.addTaskComment).toHaveBeenCalledWith('', 'tok', expect.any(String), { body: 'Ок', files: [], internId: 'intern-1' }));
  });

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
    const dialog = await openReviewModal();
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
    const dialog = await openReviewModal();
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

  it('пока отправок нет, блока «История отправок» нет', async () => {
    api.getSubmissionHistory.mockResolvedValue([]);
    const dialog = await openModal();
    await vi.waitFor(() => expect(api.getSubmissionHistory).toHaveBeenCalled());
    expect(within(dialog).queryByRole('button', { name: 'История отправок' })).toBeNull();
    expect(within(dialog).queryByText('Отправок пока нет.')).toBeNull();
  });

  it('если история не загрузилась, ссылка остаётся и при раскрытии повторяет запрос (с сообщением и без)', async () => {
    api.getSubmissionHistory.mockRejectedValueOnce(new Error('Нет истории')).mockRejectedValueOnce('boom');
    const dialog = await openModal();
    await userEvent.click(await within(dialog).findByRole('button', { name: 'История отправок' }));
    expect(await within(dialog).findByText('Не удалось загрузить историю')).toBeInTheDocument();
    expect(api.getSubmissionHistory).toHaveBeenCalledTimes(2);
    await userEvent.click(within(dialog).getByRole('button', { name: 'Скрыть историю' }));
    await vi.waitFor(() => expect(within(dialog).getByRole('button', { name: 'История отправок' })).toBeInTheDocument());
  });

  it('наставнику блок истории показывается только при наличии отправок', async () => {
    api.getInternSubmissionHistory.mockResolvedValueOnce([]);
    const { unmount } = render(<TaskModal item={item({ status: 'completed', requires_review: true })} token="tok" internId="intern-1" onClose={vi.fn()} />);
    await vi.waitFor(() => expect(api.getInternSubmissionHistory).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('region', { name: 'История отправок' })).toBeNull();
    unmount();
    const entry = { id: 'h1', progress_id: 'p', attempt_number: 1, result_text: 'Отчёт', result_url: null, status: 'pending', submitted_at: '2026-11-14T10:00:00Z', reviewed_at: null, review_comment: null } as const;
    api.getInternSubmissionHistory.mockResolvedValue([entry]);
    render(<TaskModal item={item({ status: 'completed', requires_review: true })} token="tok" internId="intern-1" onClose={vi.fn()} />);
    expect(await screen.findByRole('region', { name: 'История отправок' })).toBeInTheDocument();
    expect(await screen.findByText(/Отчёт/)).toBeInTheDocument();
  });
});

describe('Возврат на доработку', () => {
  const reworked = () => checklist({ stages: [{ id: 's', title: 'Этап', week_start: 1, week_end: 1, goal: '', expected_result: '', position: 1,
    items: [item({ title: 'Вернули', status: 'active', requires_review: true, returned_for_rework: true, rework_comment: 'Добавьте ссылку на PR' }), item({ title: 'Обычная', status: 'active' })] }] });

  it('карточка в «Активно» помечена «На доработку», комментарий виден в окне задачи', async () => {
    render(<KanbanBoard checklist={reworked()} token="tok" refresh={vi.fn()} />);
    const cards = within(column('Активно')).getAllByRole('article');
    expect(within(cards[0]!).getByText('На доработку')).toBeInTheDocument();
    expect(within(cards[1]!).queryByText('На доработку')).not.toBeInTheDocument();
    await userEvent.click(within(cards[0]!).getByRole('button', { name: /Открыть задачу/ }));
    expect(await screen.findByText('Добавьте ссылку на PR')).toBeInTheDocument();
  });
});
