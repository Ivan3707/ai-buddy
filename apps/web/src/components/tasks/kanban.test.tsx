import { render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { checklist, item } from '../../test/fixtures';
import { KanbanBoard } from './KanbanBoard';

describe('KanbanBoard', () => {
  it('раскладывает задачи по колонкам статусов', () => {
    const data = checklist();
    data.stages[0].items.push(item({ title: 'Проверенная задача', status: 'verified' }));
    render(<KanbanBoard checklist={data} token="tok" refresh={vi.fn()} />);
    const titles = screen.getAllByRole('heading', { level: 3 }).map(h => h.textContent);
    expect(titles).toEqual(['Недоступно', 'Активно', 'Завершено', 'Проверено']);
    expect(within(screen.getByRole('region', { name: 'Недоступно' })).getByText('Третья задача')).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Активно' })).getByText('Вторая задача')).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Завершено' })).getByText('Первая задача')).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Проверено' })).getByText('Проверенная задача')).toBeInTheDocument();
  });

  it('у активной задачи есть кнопка «Завершить», пустая колонка помечена', () => {
    render(<KanbanBoard checklist={checklist()} token="tok" refresh={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Завершить' })).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Проверено' })).getByText('Пусто')).toBeInTheDocument();
  });
});
