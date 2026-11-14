import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApiError } from '@ai-buddy/client';
import * as client from '@ai-buddy/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import { buddyUser, checklistsResponse, internUser } from './test/fixtures';

// Бади (виджет) ходит в сеть сам и проверяется отдельно: здесь он не нужен
vi.mock('./bot/BuddyHost', () => ({ BuddyHost: () => null }));

vi.mock('@ai-buddy/client', async importOriginal => {
  const actual = await importOriginal<typeof import('@ai-buddy/client')>();
  return {
    ...actual,
    login: vi.fn(),
    getMe: vi.fn(),
    getChecklists: vi.fn(),
    chat: vi.fn(),
    getPendingSubmissions: vi.fn(),
    refreshSession: vi.fn(),
    logoutSession: vi.fn(),
    getAssignedInterns: vi.fn(),
    getInternProgress: vi.fn(),
    getFeedback: vi.fn(),
    submitFeedback: vi.fn(),
  };
});

const api = vi.mocked(client);
const AUTH_KEY = 'ai-buddy:auth';
const stored = () => JSON.parse(localStorage.getItem(AUTH_KEY) ?? 'null') as { access: string } | null;
/** Сохранённая сессия: вход переживает закрытие вкладки и браузера. */
const saveSession = (access = 'saved') => localStorage.setItem(AUTH_KEY, JSON.stringify({
  access, refresh: 'refresh', expiresAt: Date.now() + 3_600_000, sub: 'u-intern', loginAt: Date.now(),
}));
const loginResponse = { access_token: 'tok', refresh_token: 'refresh', token_type: 'Bearer', expires_in: 3600, is_first_login: false };

async function signIn(email = 'spike@example.com', password = 'secret-password') {
  await userEvent.type(screen.getByLabelText('Логин, почта, номер телефона'), email);
  await userEvent.type(screen.getByLabelText('Пароль'), password);
  await userEvent.click(screen.getByRole('button', { name: 'Войти' }));
}

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  api.logoutSession.mockResolvedValue(undefined);
  api.getFeedback.mockResolvedValue({ feedback: null });
  api.submitFeedback.mockResolvedValue({} as never);
  api.refreshSession.mockRejectedValue(new ApiError(401, 'revoked'));
  api.getChecklists.mockResolvedValue(checklistsResponse());
  api.getPendingSubmissions.mockResolvedValue([]);
});

describe('вход', () => {
  it('показывает форму входа без сохранённой сессии', () => {
    render(<App />);
    expect(screen.getByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });

  it('стажёр входит и попадает на главную', async () => {
    api.login.mockResolvedValue({ ...loginResponse, user: internUser });
    render(<App />);
    await signIn();
    expect(await screen.findByRole('heading', { name: 'Привет, Spike!' })).toBeInTheDocument();
    expect(api.login).toHaveBeenCalledWith('', 'spike@example.com', 'secret-password');
    expect(stored()?.access).toBe('tok');
    expect(api.getMe).not.toHaveBeenCalled();
  });

  it('при первом входе стажёра показывает приветствие, после «Пропустить» — больше нет', async () => {
    localStorage.clear();
    api.login.mockResolvedValue({ ...loginResponse, access_token: 'tok', token_type: 'Bearer', expires_in: 3600, is_first_login: true, user: internUser });
    render(<App />);
    await signIn();
    expect(await screen.findByRole('heading', { name: 'Добро пожаловать на стажировку в Авито!' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Пропустить' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(localStorage.getItem(`onboarding:${internUser.id}`)).toBeNull();
  });

  it('при обычном входе приветствия нет', async () => {
    localStorage.clear();
    api.login.mockResolvedValue({ ...loginResponse, access_token: 'tok', token_type: 'Bearer', expires_in: 3600, is_first_login: false, user: internUser });
    render(<App />);
    await signIn();
    await screen.findByRole('heading', { name: 'Привет, Spike!' });
    expect(screen.queryByRole('heading', { name: 'Добро пожаловать на стажировку в Авито!' })).not.toBeInTheDocument();
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
    saveSession('saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    expect(screen.getByText('Восстанавливаем сессию…')).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: /Привет/ })).toBeInTheDocument();
    expect(api.getMe).toHaveBeenCalledWith('', 'saved');
  });

  it('сбрасывает токен при 401', async () => {
    saveSession('expired');
    api.getMe.mockRejectedValue(new ApiError(401, 'unauthorized'));
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(stored()).toBeNull();
  });

  it('показывает ошибку при сбое сети и остаётся на входе', async () => {
    saveSession('saved');
    api.getMe.mockRejectedValue(new Error('Сеть недоступна'));
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Сеть недоступна');
  });

  it('использует запасной текст для неизвестной ошибки', async () => {
    saveSession('saved');
    api.getMe.mockRejectedValue('boom');
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось восстановить сессию');
  });

  it('выходит при событии unauthorized', async () => {
    saveSession('saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    await screen.findByRole('heading', { name: /Привет/ });
    globalThis.dispatchEvent(new Event('ai-buddy:unauthorized'));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });
});

describe('кабинет стажёра', () => {
  async function openShell() {
    saveSession('saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    await screen.findByRole('heading', { name: /Привет/ });
  }

  it('главная: переключает выбранный чек-лист и открывает задачи по клику на строку', async () => {
    await openShell();
    expect(await screen.findByText('1/3 выполнено')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Профессиональный чек-лист/ }));
    expect(screen.getByText('0/1 выполнено')).toBeInTheDocument();
    await userEvent.click(screen.getByText('Доступ к репозиторию'));
    expect(await screen.findByRole('heading', { name: 'Задачи' })).toBeInTheDocument();
    expect(await screen.findByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /^Общий\s*чек-лист$/ }));
    expect(await screen.findByRole('region', { name: 'Общий чек-лист' })).toBeInTheDocument();
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

  it('выход из аккаунта — значок внизу бокового меню', async () => {
    await openShell();
    await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });

  it('заметки: вкладка открывает панель и закрывается по Esc', async () => {
    await openShell();
    await userEvent.click(screen.getByRole('button', { name: 'Заметки' }));
    const dialog = screen.getByRole('dialog', { name: 'Заметки' });
    expect(within(dialog).getByRole('heading', { name: 'Заметки' })).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('обратная связь не показывается, пока чек-листы не завершены', async () => {
    await openShell();
    expect(screen.queryByRole('heading', { name: /Есть идеи/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Обратная связь' })).not.toBeInTheDocument();
  });

  it('обратная связь появляется после завершения обоих чек-листов: отправка и сообщение об успехе', async () => {
    const done = checklistsResponse();
    api.getChecklists.mockResolvedValue({
      common: { ...done.common, completion_percent: 100 },
      professional: { ...done.professional, completion_percent: 100 },
    });
    saveSession('saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    expect(await screen.findByRole('heading', { name: /Есть идеи/ })).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Опиши свою идею или задай вопрос'), 'Добавьте тёмную тему');
    await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Поставьте оценку');
    await userEvent.click(screen.getByRole('button', { name: 'Оценка 5 из 5' }));
    api.getFeedback.mockResolvedValue({ feedback: { id: 'f1', user_id: 'u', checklist_id: 'c', rating: 5, comment: '', delivery_status: 'sent', created_at: '', delivered_at: null } });
    await userEvent.click(screen.getByRole('button', { name: 'Отправить' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Спасибо! Отзыв отправлен.');
    expect(api.submitFeedback).toHaveBeenCalledTimes(2);
    expect(api.submitFeedback).toHaveBeenCalledWith('', 'saved', expect.any(String), { rating: 5, comment: 'Добавьте тёмную тему' });
  });

  it('при ошибке загрузки чек-листов даёт повторить', async () => {
    api.getChecklists.mockRejectedValueOnce(new Error('Сервер недоступен')).mockResolvedValue(checklistsResponse());
    await (async () => {
      saveSession('saved');
      api.getMe.mockResolvedValue(internUser);
      render(<App />);
    })();
    expect(await screen.findByRole('alert')).toHaveTextContent('Сервер недоступен');
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }));
    expect(await screen.findByText('1/3 выполнено')).toBeInTheDocument();
  });

  it('использует запасной текст при неизвестной ошибке загрузки', async () => {
    api.getChecklists.mockRejectedValue('boom');
    saveSession('saved');
    api.getMe.mockResolvedValue(internUser);
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось загрузить чек-листы');
  });
});

describe('кабинет наставника', () => {
  it('наставник видит сводку и меню в том же оформлении и может выйти', async () => {
    saveSession('saved');
    api.getMe.mockResolvedValue(buddyUser);
    api.getAssignedInterns.mockResolvedValue([internUser]);
    api.getInternProgress.mockResolvedValue({
      user_id: internUser.id, total_items: 4, completed_items: 1, completion_percent: 25,
      common: checklistsResponse().common, professional: checklistsResponse().professional,
    });
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Привет, Buddy!' })).toBeInTheDocument();
    expect(await screen.findByText('Ждут проверки')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Команда' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Вопросы' }));
    expect(await screen.findByRole('heading', { name: 'Вопросы' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Бади' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(stored()).toBeNull();
  });
});
