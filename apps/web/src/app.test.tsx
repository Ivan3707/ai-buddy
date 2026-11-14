import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApiError } from '@ai-buddy/client';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import App, { TOKEN_KEY } from './App';
import { buddyUser, checklistsResponse, internUser } from './test/fixtures';

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual,
    login: vi.fn(),
    getMe: vi.fn(),
    getChecklists: vi.fn(),
    chat: vi.fn(),
    getPendingSubmissions: vi.fn(),
  };
});

const api = vi.mocked(client);

async function signIn(email = 'spike@example.com', password = 'secret-password') {
  await userEvent.type(screen.getByLabelText('Логин, почта, номер телефона'), email);
  await userEvent.type(screen.getByLabelText('Пароль'), password);
  await userEvent.click(screen.getByRole('button', { name: 'Войти' }));
}

beforeEach(() => {
  vi.resetAllMocks();
  api.getChecklists.mockResolvedValue(checklistsResponse());
  api.getPendingSubmissions.mockResolvedValue([]);
});

describe('вход', () => {
  it('показывает форму входа без сохранённой сессии', () => {
    render(<App />);
    expect(screen.getByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });

  it('стажёр входит и попадает на главную', async () => {
    api.login.mockResolvedValue({ access_token: 'tok', token_type: 'Bearer', expires_in: 3600, is_first_login: false, user: internUser });
    render(<App />);
    await signIn();
    expect(await screen.findByRole('heading', { name: 'Привет, Spike!' })).toBeInTheDocument();
    expect(api.login).toHaveBeenCalledWith('', 'spike@example.com', 'secret-password');
    expect(sessionStorage.getItem(TOKEN_KEY)).toBe('tok');
    expect(api.getMe).not.toHaveBeenCalled();
  });

  it('показывает ошибку неверного пароля и блокирует кнопку на время запроса', async () => {
    api.login.mockRejectedValue(new ApiError(401, 'Неверный email или пароль'));
    render(<App />);
    await signIn();
    expect(await screen.findByRole('alert')).toHaveTextContent('Неверный email или пароль');
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled();
  });

  it('подставляет запасной текст, если ошибка без сообщения', async () => {
    api.login.mockRejectedValue('boom');
    render(<App />);
    await signIn();
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось войти');
  });
});

describe('восстановление сессии', () => {
  it('восстанавливает пользователя по токену', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    expect(screen.getByText('Восстанавливаем сессию…')).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: /Привет/ })).toBeInTheDocument();
    expect(api.getMe).toHaveBeenCalledWith('', 'saved');
  });

  it('сбрасывает токен при 401', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'expired');
    api.getMe.mockRejectedValue(new ApiError(401, 'unauthorized'));
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(sessionStorage.getItem(TOKEN_KEY)).toBeNull();
  });

  it('показывает ошибку при сбое сети и остаётся на входе', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockRejectedValue(new Error('Сеть недоступна'));
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Сеть недоступна');
  });

  it('использует запасной текст для неизвестной ошибки', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockRejectedValue('boom');
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось восстановить сессию');
  });

  it('выходит при событии unauthorized', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    await screen.findByRole('heading', { name: /Привет/ });
    globalThis.dispatchEvent(new Event('ai-buddy:unauthorized'));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });
});

describe('кабинет стажёра', () => {
  async function openShell() {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    await screen.findByRole('heading', { name: /Привет/ });
  }

  it('главная: переключает выбранный чек-лист и открывает задачи по клику на строку', async () => {
    await openShell();
    expect(screen.getByText('1/3 выполнено')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Профессиональный чек-лист/ }));
    expect(screen.getByText('0/1 выполнено')).toBeInTheDocument();
    await userEvent.click(screen.getByText('Доступ к репозиторию'));
    expect(await screen.findByRole('heading', { name: 'Задачи' })).toBeInTheDocument();
    expect(await screen.findByRole('region', { name: 'Общий чек-лист' })).toBeInTheDocument();
    expect(await screen.findByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
  });

  it('навигация: календарь, задачи, главная', async () => {
    await openShell();
    await userEvent.click(screen.getByRole('button', { name: 'Календарь' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Календарь' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Задачи' }));
    expect(await screen.findByRole('region', { name: 'Общий чек-лист' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Главная' }));
    expect(screen.getByRole('heading', { name: /Привет/ })).toBeInTheDocument();
  });

  it('профиль открывается и закрывается по клику на фон', async () => {
    await openShell();
    await userEvent.click(screen.getByRole('button', { name: 'Профиль' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('25 %')).toBeInTheDocument();
    expect(within(dialog).getByText('1/4')).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Выйти' })).not.toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('обратная связь: отправка и сообщение об успехе', async () => {
    await openShell();
    await userEvent.click(screen.getByRole('button', { name: 'Обратная связь' }));
    expect(screen.getByRole('heading', { name: /Есть идеи/ })).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Опиши свою идею или задай вопрос'), 'Добавьте тёмную тему');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Сообщение отправлено');
  });

  it('при ошибке загрузки чек-листов даёт повторить', async () => {
    api.getChecklists.mockRejectedValueOnce(new Error('Сервер недоступен')).mockResolvedValue(checklistsResponse());
    await (async () => {
      sessionStorage.setItem(TOKEN_KEY, 'saved');
      api.getMe.mockResolvedValue(internUser);
      render(<App />);
    })();
    expect(await screen.findByRole('alert')).toHaveTextContent('Сервер недоступен');
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }));
    expect(await screen.findByText('1/3 выполнено')).toBeInTheDocument();
  });

  it('использует запасной текст при неизвестной ошибке загрузки', async () => {
    api.getChecklists.mockRejectedValue('boom');
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось загрузить чек-листы');
  });
});

describe('AI-помощник', () => {
  async function openAssistant() {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    await screen.findByRole('heading', { name: /Привет/ });
    await userEvent.click(screen.getByRole('button', { name: 'Спросить AI-помощника' }));
    return screen.getByRole('region', { name: 'AI-помощник' });
  }

  it('отправляет вопрос и показывает ответ с источником', async () => {
    api.chat.mockResolvedValue({ answer: 'Подайте заявку', sources: [{ id: 'leave', title: 'Заявление на отпуск', url: 'https://hr.example.com/leave' }], mode: 'mock' });
    const assistant = await openAssistant();
    await userEvent.type(within(assistant).getByLabelText('Ваш вопрос'), 'Как оформить отпуск?');
    await userEvent.click(within(assistant).getByRole('button', { name: 'Отправить' }));
    expect(await within(assistant).findByText('Подайте заявку')).toBeInTheDocument();
    expect(within(assistant).getByRole('link', { name: 'Заявление на отпуск' })).toHaveAttribute('href', 'https://hr.example.com/leave');
    expect(api.chat).toHaveBeenCalledWith('', 'Как оформить отпуск?', 'saved');
  });

  it('показывает ошибку, если помощник недоступен', async () => {
    api.chat.mockRejectedValue(new Error('AI недоступен'));
    const assistant = await openAssistant();
    await userEvent.type(within(assistant).getByLabelText('Ваш вопрос'), 'Привет');
    await userEvent.click(within(assistant).getByRole('button', { name: 'Отправить' }));
    expect(await within(assistant).findByRole('alert')).toHaveTextContent('AI недоступен');
  });

  it('использует запасной текст и сворачивается', async () => {
    api.chat.mockRejectedValue('boom');
    const assistant = await openAssistant();
    await userEvent.type(within(assistant).getByLabelText('Ваш вопрос'), 'Привет');
    await userEvent.click(within(assistant).getByRole('button', { name: 'Отправить' }));
    expect(await within(assistant).findByRole('alert')).toHaveTextContent('Помощник недоступен');
    await userEvent.click(within(assistant).getByRole('button', { name: 'Спросить AI-помощника' }));
    await waitFor(() => expect(within(assistant).queryByLabelText('Ваш вопрос')).toBeNull());
  });
});

describe('кабинет buddy', () => {
  it('buddy видит прежний кабинет с очередью проверок и может выйти', async () => {
    sessionStorage.setItem(TOKEN_KEY, 'saved');
    api.getMe.mockResolvedValue(buddyUser);
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Очередь проверок' })).toBeInTheDocument();
    expect(screen.getByText('Buddy', { selector: '.role' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });
});
