import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { internUser } from '../../test/fixtures';
import { ProfileDrawer } from './ProfileDrawer';
import { SideTabs } from './SideTabs';
import { Sidebar } from './Sidebar';

describe('Sidebar', () => {
  it('отмечает текущий раздел и переключает разделы', async () => {
    const onNavigate = vi.fn();
    render(<Sidebar view="home" onNavigate={onNavigate} />);
    expect(screen.getByRole('button', { name: 'Главная' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('button', { name: 'Задачи' })).not.toHaveAttribute('aria-current');
    await userEvent.click(screen.getByRole('button', { name: 'Календарь' }));
    expect(onNavigate).toHaveBeenCalledWith('calendar');
  });
});

describe('SideTabs', () => {
  it('открывает профиль и обратную связь', async () => {
    const onProfile = vi.fn();
    const onFeedback = vi.fn();
    render(<SideTabs onProfile={onProfile} onFeedback={onFeedback} />);
    await userEvent.click(screen.getByRole('button', { name: 'Профиль' }));
    await userEvent.click(screen.getByRole('button', { name: 'Обратная связь' }));
    expect(onProfile).toHaveBeenCalled();
    expect(onFeedback).toHaveBeenCalled();
  });
});

describe('ProfileDrawer', () => {
  const progress = { completed: 1, total: 20, active: 2, percent: 5 };

  it('ничего не рисует, пока закрыт', () => {
    render(<ProfileDrawer user={internUser} progress={progress} open={false} onClose={() => undefined} onLogout={() => undefined} />);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('показывает личный кабинет: ФИО, стаж, прогресс и данные о себе', () => {
    render(<ProfileDrawer user={internUser} progress={progress} open onClose={() => undefined} onLogout={() => undefined} />);
    const dialog = screen.getByRole('dialog', { name: 'Профиль' });
    expect(within(dialog).getByRole('heading', { name: 'Личный кабинет' })).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: /Кузнецов/ })).toBeInTheDocument();
    expect(within(dialog).getByText('25 лет')).toBeInTheDocument();
    expect(within(dialog).getByText('5 %')).toBeInTheDocument();
    expect(within(dialog).getByText('1/20')).toBeInTheDocument();
    expect(within(dialog).getByText('в работе: 2')).toBeInTheDocument();
    expect(within(dialog).getByRole('progressbar', { name: 'Общий прогресс стажировки' })).toHaveAttribute('aria-valuenow', '5');
    const about = within(dialog).getByRole('region', { name: 'О себе' });
    expect(about).toHaveTextContent('Бэкенд-разработчик');
    expect(about).toHaveTextContent('Руководитель');
    expect(about).toHaveTextContent('Наставник');
    expect(dialog).toHaveTextContent(internUser.email);
  });

  it('показывает прочерк без профессии', () => {
    render(<ProfileDrawer user={{ ...internUser, profession: null }} progress={progress} open onClose={() => undefined} onLogout={() => undefined} />);
    expect(within(screen.getByRole('region', { name: 'О себе' })).getByText('—')).toBeInTheDocument();
  });

  it('закрывается и позволяет выйти', async () => {
    const onClose = vi.fn();
    const onLogout = vi.fn();
    render(<ProfileDrawer user={internUser} progress={progress} open onClose={onClose} onLogout={onLogout} />);
    await userEvent.click(screen.getByRole('dialog'));
    expect(onClose).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть профиль' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
