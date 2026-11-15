import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { checklist, checklistsResponse, item } from '../../test/fixtures';
import { ChecklistCard } from './ChecklistCard';
import { MiniCalendar } from './MiniCalendar';
import { ProgressRing } from './ProgressRing';
import { TaskTable } from './TaskTable';

describe('ProgressRing', () => {
  it('показывает процент и ограничивает значение 0–100', () => {
    const { rerender } = render(<ProgressRing percent={42.4} label="Прогресс" />);
    expect(screen.getByRole('progressbar', { name: 'Прогресс' })).toHaveAttribute('aria-valuenow', '42');
    expect(screen.getByText('42%')).toBeInTheDocument();
    rerender(<ProgressRing percent={150} label="Прогресс" />);
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100');
    rerender(<ProgressRing percent={-5} label="Прогресс" />);
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0');
  });
});

describe('ChecklistCard', () => {
  it('показывает название без пометки версии, срок этапа и прогресс', () => {
    render(<ChecklistCard checklist={checklist()} selected onSelect={() => undefined} />);
    const region = screen.getByRole('region', { name: 'Общий чек-лист' });
    expect(within(region).getByText('Основной чек-лист: Адаптация в команде')).toBeInTheDocument();
    expect(within(region).getByText('1-2 неделя')).toBeInTheDocument();
    expect(within(region).getByRole('progressbar')).toHaveAttribute('aria-valuenow', '33');
  });

  it('вызывает onSelect по клику и подписывает профессиональный трек', async () => {
    const onSelect = vi.fn();
    render(<ChecklistCard checklist={checklistsResponse().professional} selected={false} onSelect={onSelect} />);
    expect(screen.getByRole('region', { name: 'Профессиональный чек-лист' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Профессиональный чек-лист/ }));
    expect(onSelect).toHaveBeenCalled();
  });

  it('не падает, если у чек-листа нет этапов', () => {
    render(<ChecklistCard checklist={checklist({ stages: [] })} selected={false} onSelect={() => undefined} />);
    expect(screen.getByRole('region', { name: 'Общий чек-лист' })).toBeInTheDocument();
  });
});

describe('MiniCalendar', () => {
  const today = new Date(2026, 10, 14);

  it('подсвечивает сегодняшний день и показывает месяц и год', () => {
    render(<MiniCalendar today={today} />);
    expect(screen.getByText('Ноябрь 2026 г.')).toBeInTheDocument();
    expect(screen.getByText('14')).toHaveClass('is-today');
    expect(screen.getByText('30')).not.toHaveClass('is-today');
  });

  it('листает месяцы и подписывает не текущий месяц', async () => {
    render(<MiniCalendar today={today} />);
    await userEvent.click(screen.getByRole('button', { name: 'Следующий месяц' }));
    expect(screen.getByText('Декабрь 2026 г.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Предыдущий месяц' }));
    expect(screen.getByText('Ноябрь 2026 г.')).toBeInTheDocument();
  });

  it('работает без переданной даты', () => {
    render(<MiniCalendar />);
    expect(screen.getByRole('region', { name: 'Календарь' })).toBeInTheDocument();
  });
});

describe('TaskTable', () => {
  it('показывает задачи текущего этапа, счётчик и заглушки недоступных', () => {
    render(<TaskTable checklist={checklist()} onOpen={() => undefined} />);
    expect(screen.getByText('1/3 выполнено')).toBeInTheDocument();
    expect(screen.getByText('Вторая задача')).toBeInTheDocument();
    expect(screen.getAllByText('Недоступно')).toHaveLength(3);
    expect(screen.getAllByText(/Описание задачи/)).toHaveLength(2);
  });

  it('показывает руководителя из профиля или прочерк', () => {
    render(<TaskTable checklist={checklist()} onOpen={() => undefined} managerName="Новикова Буся" />);
    expect(screen.getAllByText('Новикова Буся').length).toBeGreaterThan(0);
  });

  it('открывает окно задачи по клику на название, а не на другие ячейки; недоступные не открываются', () => {
    const onOpen = vi.fn();
    render(<TaskTable checklist={checklist()} onOpen={onOpen} />);
    fireEvent.click(screen.getAllByText(/Описание задачи/)[0]!);
    fireEvent.click(screen.getByText('Третья задача'));
    expect(onOpen).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('Вторая задача'));
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(onOpen.mock.calls[0]![0]).toMatchObject({ title: 'Вторая задача' });
    fireEvent.keyDown(screen.getByText('Вторая задача'), { key: 'Enter' });
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it('сообщает, когда задач нет', () => {
    render(<TaskTable checklist={checklist({ stages: [] })} onOpen={() => undefined} />);
    expect(screen.getByText('0/0 выполнено')).toBeInTheDocument();
    expect(screen.getByText('В этом чек-листе пока нет задач.')).toBeInTheDocument();
  });

  it('помечает выполненные задачи', () => {
    const done = checklist();
    done.stages[0]!.items = [item({ status: 'verified', title: 'Готово' })];
    const { container } = render(<TaskTable checklist={done} onOpen={() => undefined} />);
    expect(container.querySelector('.task-row.is-done')).not.toBeNull();
  });
});
