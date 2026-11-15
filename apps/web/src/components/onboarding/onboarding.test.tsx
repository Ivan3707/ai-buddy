import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { TOUR } from './content';
import { Onboarding, botBox, placeBubble } from './Onboarding';

describe('Onboarding', () => {
  it('приветствие содержит текст и четыре возможности', () => {
    render(<Onboarding onNavigate={vi.fn()} onDone={vi.fn()} />);
    expect(screen.getByRole('heading', { name: 'Добро пожаловать на стажировку в Авито!' })).toBeInTheDocument();
    for (const title of ['Следить за задачами', 'Оценивать свой прогресс', 'Сохранять результаты', 'Работать над развитием']) {
      expect(screen.getByText(title)).toBeInTheDocument();
    }
  });

  it('«Пропустить» на приветствии завершает онбординг', async () => {
    const onDone = vi.fn();
    render(<Onboarding onNavigate={vi.fn()} onDone={onDone} />);
    await userEvent.click(screen.getByRole('button', { name: 'Пропустить' }));
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('проходит все подсказки, переключает страницы и завершается кнопкой «Начать»', async () => {
    const onNavigate = vi.fn();
    const onDone = vi.fn();
    render(<Onboarding onNavigate={onNavigate} onDone={onDone} />);
    await userEvent.click(screen.getByRole('button', { name: 'Показать, как всё устроено' }));
    for (let step = 0; step < TOUR.length - 1; step += 1) {
      expect(screen.getByRole('heading', { name: TOUR[step].title })).toBeInTheDocument();
      expect(screen.getByText(`${step + 1} из ${TOUR.length}`)).toBeInTheDocument();
      await userEvent.click(screen.getByRole('button', { name: 'Далее' }));
    }
    expect(screen.getByRole('heading', { name: 'Личная информация' })).toBeInTheDocument();
    expect(onNavigate).toHaveBeenCalledWith('tasks', 'professional');
    // после перехода в задачи возврата на главную нет
    expect(onNavigate.mock.calls.slice(-1)[0][0]).toBe('tasks');
    await userEvent.click(screen.getByRole('button', { name: 'Назад' }));
    expect(screen.getByRole('heading', { name: 'Что умеет Бади' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Далее' }));
    await userEvent.click(screen.getByRole('button', { name: 'Начать' }));
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('Esc во время тура завершает онбординг', async () => {
    const onDone = vi.fn();
    render(<Onboarding onNavigate={vi.fn()} onDone={onDone} />);
    await userEvent.click(screen.getByRole('button', { name: 'Показать, как всё устроено' }));
    await userEvent.keyboard('{Escape}');
    expect(onDone).toHaveBeenCalledTimes(1);
  });
});

describe('placeBubble', () => {
  const area = { width: 1920, height: 1080 };
  const bubble = { width: 400, height: 260 };

  it('ставит справа от элемента, если там есть место', () => {
    const pos = placeBubble({ top: 200, left: 300, width: 400, height: 300 }, bubble, area);
    expect(pos.left).toBe(728);
  });

  it('у правого края уходит влево и не вылезает за экран', () => {
    const pos = placeBubble({ top: 20, left: 1840, width: 80, height: 150 }, bubble, area);
    expect(pos.left).toBe(1840 - 28 - 400);
    expect(pos.top).toBeGreaterThanOrEqual(16);
  });

  it('если по бокам тесно, ставит под или над элементом', () => {
    const wide = placeBubble({ top: 100, left: 100, width: 1700, height: 200 }, bubble, area);
    expect(wide.top).toBe(328);
    const low = placeBubble({ top: 800, left: 100, width: 1700, height: 200 }, bubble, area);
    expect(low.top).toBe(800 - 28 - 260);
  });
});

describe('botBox', () => {
  it('без виджета берёт правый нижний угол', () => {
    expect(botBox(1000, 800, 1, null)).toEqual({ top: 800 - 72 - 24 - 18, left: 1000 - 72 - 24 - 18, width: 108, height: 108 });
  });

  it('следует за текущим положением Бади и учитывает zoom', () => {
    const host = document.createElement('ai-buddy-character');
    host.dataset.buddyX = '100'; host.dataset.buddyY = '50'; host.dataset.buddySize = '72';
    expect(botBox(1000, 800, 1, host)).toEqual({ top: 32, left: 82, width: 108, height: 108 });
    expect(botBox(1000, 800, 2, host)).toEqual({ top: 16, left: 41, width: 54, height: 54 });
  });
});

describe('TOUR', () => {
  it('шаг про обратную связь показывает колонку «Активно»', () => {
    const step = TOUR.find(item => item.title === 'Обратная связь');
    expect(step?.targets).toEqual(['.kanban-column.is-active']);
    expect(step?.text.join(' ')).toContain('На доработку');
  });
});
