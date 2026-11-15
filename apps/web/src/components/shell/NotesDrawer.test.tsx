import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NotesDrawer } from './NotesDrawer';

const api = vi.hoisted(() => ({
  listNotes: vi.fn(), createNote: vi.fn(), updateNote: vi.fn(), deleteNote: vi.fn(),
}));
vi.mock('@ai-buddy/client', () => api);

const note = { id: 'n1', title: 'Доступы', content: 'Запросить Grafana', updated_at: '2026-11-10T10:00:00Z' };

describe('NotesDrawer', () => {
  beforeEach(() => {
    Object.values(api).forEach(fn => fn.mockReset());
    api.listNotes.mockResolvedValue([note]);
    api.updateNote.mockResolvedValue(note);
    api.createNote.mockResolvedValue(note);
  });

  it('opens a note in read-only view and edits only after the edit button', async () => {
    render(<NotesDrawer open token="t" onClose={() => {}} />);
    await userEvent.click(await screen.findByRole('button', { name: /Доступы/ }));
    expect(screen.queryByLabelText('Заголовок')).toBeNull();
    expect(screen.getByText('Запросить Grafana', { selector: 'p' })).toBeTruthy();
    await userEvent.click(screen.getByRole('button', { name: 'Редактировать' }));
    await userEvent.clear(screen.getByLabelText('Заголовок'));
    await userEvent.type(screen.getByLabelText('Заголовок'), 'Новое');
    await userEvent.click(screen.getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(api.updateNote).toHaveBeenCalledWith('', 't', 'n1', { title: 'Новое', content: 'Запросить Grafana' }));
  });

  it('cancel returns to view without saving; new note starts in edit mode', async () => {
    render(<NotesDrawer open token="t" onClose={() => {}} />);
    await userEvent.click(await screen.findByRole('button', { name: /Доступы/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Редактировать' }));
    await userEvent.click(screen.getByRole('button', { name: 'Отмена' }));
    expect(screen.queryByLabelText('Заголовок')).toBeNull();
    expect(api.updateNote).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }));
    await userEvent.click(screen.getByRole('button', { name: 'Добавить заметку' }));
    expect(screen.getByLabelText('Заголовок')).toBeTruthy();
  });
});
