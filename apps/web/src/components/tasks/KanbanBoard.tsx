import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type PointerEvent as ReactPointerEvent } from 'react';
import type { Checklist, ChecklistItem } from '@ai-buddy/client';
import { advanceItem } from '../../lib/advance';
import { COLUMN_TITLES, columnOf, nextStepLabel, type Column } from '../../lib/taskColumns';
import { TaskModal } from './TaskModal';

type Status = Column;

/** Колонки доски: порядок задан дизайнером. */
export const KANBAN_COLUMNS: { status: Status; title: string }[] = (['unavailable', 'active', 'review', 'done'] as const).map(status => ({ status, title: COLUMN_TITLES[status] }));

/** Стажёр сам переносит активную задачу вперёд: в «На проверке» или «Завершено». Решение «проверено» принимает наставник. */
const DROP_TARGETS: Status[] = ['review', 'done'];
const DRAG_DISTANCE = 5;
const EDGE_ZONE = 56;

type Props = { checklist: Checklist; token: string; refresh: () => Promise<void> };
type Drag = { id: string; left: number; top: number; width: number; height: number; over: Status | null };
type Gesture = { id: string; startX: number; startY: number; offsetX: number; offsetY: number; rect: DOMRect; active: boolean; pointerId: number };

const INTERACTIVE = 'button, a, input, textarea, select, label, form, summary';

/**
 * Задачи одного чек-листа в виде канбан-доски.
 * Активную задачу переносят вперёд мышью, пальцем за ручку ⠿, с клавиатуры или кнопкой на карточке и в окне задачи.
 * Задача без проверки уходит в «Завершено» (колонку «На проверке» она проскакивает),
 * задача с проверкой попадает в «На проверке», и там её принимает наставник.
 */
export function KanbanBoard({ checklist, token, refresh }: Props) {
  const [busyID, setBusyID] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [drag, setDrag] = useState<Drag | null>(null);
  const [picked, setPicked] = useState<{ id: string; over: boolean } | null>(null);
  const [openID, setOpenID] = useState('');
  const justDragged = useRef(false);
  const gesture = useRef<Gesture | null>(null);
  const pointer = useRef({ x: 0, y: 0 });
  const frame = useRef(0);
  const columns = useRef<HTMLDivElement>(null);
  const trackName = checklist.type === 'common' ? 'Общий' : 'Профессиональный';
  const cards = checklist.stages.flatMap(stage => stage.items.map(item => ({ item, stage })));
  const byID = useCallback((id: string) => cards.find(card => card.item.id === id)?.item, [cards]);

  const opened = cards.find(card => card.item.id === openID);

  /**
   * Следующий этап. Задача без проверки завершается сразу.
   * Задачу с проверкой сразу не отправляем: открываем окно, где стажёр оставляет комментарий и ссылку на результат.
   */
  const advance = async (item: ChecklistItem) => {
    if (item.requires_review) {
      setError('');
      setNotice(`«${item.title}» принимает наставник: добавьте комментарий или ссылку на результат и отправьте на проверку.`);
      setOpenID(item.id);
      return;
    }
    setBusyID(item.id);
    setError('');
    setNotice('');
    try {
      await advanceItem(token, item);
      await refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось завершить задачу');
    } finally {
      setBusyID('');
    }
  };

  /** Что делает отпускание карточки над колонкой `target`. */
  const drop = (item: ChecklistItem, target: Status | null) => {
    setError('');
    if (target === null || target === columnOf(item)) { setNotice(''); return; }
    if (!DROP_TARGETS.includes(target)) {
      setNotice('Карточку можно перенести только вперёд: «На проверке» или «Завершено».');
      return;
    }
    setNotice('');
    void advance(item);
  };

  const overColumn = (x: number, y: number): Status | null => {
    const hit = document.elementsFromPoint(x, y).find(node => node instanceof HTMLElement && node.dataset.status);
    return (hit as HTMLElement | undefined)?.dataset.status as Status | null ?? null;
  };

  const stop = () => {
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onCancel);
    cancelAnimationFrame(frame.current);
    gesture.current = null;
    setDrag(null);
  };

  // На узких экранах колонки листаются вбок: у края карточка сама подкручивает доску, чтобы дотянуться до «Завершено».
  const loop = () => {
    const g = gesture.current;
    const area = columns.current;
    if (!g?.active) return;
    if (area) {
      const box = area.getBoundingClientRect();
      if (pointer.current.x < box.left + EDGE_ZONE) area.scrollLeft -= 14;
      else if (pointer.current.x > box.right - EDGE_ZONE) area.scrollLeft += 14;
    }
    setDrag(d => (d ? { ...d, over: overColumn(pointer.current.x, pointer.current.y) } : d));
    frame.current = requestAnimationFrame(loop);
  };

  function onMove(event: PointerEvent) {
    const g = gesture.current;
    if (!g || event.pointerId !== g.pointerId) return;
    pointer.current = { x: event.clientX, y: event.clientY };
    if (!g.active) {
      if (Math.hypot(event.clientX - g.startX, event.clientY - g.startY) < DRAG_DISTANCE) return;
      g.active = true;
      setNotice('');
      frame.current = requestAnimationFrame(loop);
    }
    setDrag({
      id: g.id, left: event.clientX - g.offsetX, top: event.clientY - g.offsetY, width: g.rect.width, height: g.rect.height,
      over: overColumn(event.clientX, event.clientY),
    });
  }
  function onUp(event: PointerEvent) {
    const g = gesture.current;
    if (!g || event.pointerId !== g.pointerId) return;
    const item = byID(g.id);
    const wasDragging = g.active;
    const target = wasDragging ? overColumn(event.clientX, event.clientY) : null;
    stop();
    if (wasDragging) { justDragged.current = true; setTimeout(() => { justDragged.current = false; }, 0); }
    if (wasDragging && item) drop(item, target);
  }
  function onCancel(event: PointerEvent) {
    if (gesture.current && event.pointerId === gesture.current.pointerId) stop();
  }

  useEffect(() => stop, []); // eslint-disable-line react-hooks/exhaustive-deps

  const openCard = (event: MouseEvent<HTMLElement>, item: ChecklistItem) => {
    if (justDragged.current || picked || (event.target as HTMLElement).closest(INTERACTIVE)) return;
    setOpenID(item.id);
  };

  const begin = (event: ReactPointerEvent<HTMLElement>, item: ChecklistItem) => {
    if (item.status !== 'active' || busyID) return;
    if (event.pointerType === 'mouse' && event.button !== 0) return;
    const target = event.target as HTMLElement;
    const fromHandle = Boolean(target.closest('.kanban-handle'));
    if (!fromHandle && target.closest(INTERACTIVE)) return; // поля формы и кнопки на карточке не тянут её
    if (event.pointerType !== 'mouse' && !fromHandle) return; // пальцем тянут только за ручку: иначе не пролистать доску
    const rect = event.currentTarget.getBoundingClientRect();
    gesture.current = {
      id: item.id, startX: event.clientX, startY: event.clientY, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top,
      rect, active: false, pointerId: event.pointerId,
    };
    pointer.current = { x: event.clientX, y: event.clientY };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onCancel);
  };

  // Клавиатура: Пробел или Enter берёт карточку, стрелка вправо переносит вперёд, Пробел отпускает, Esc отменяет.
  const onKey = (event: KeyboardEvent<HTMLElement>, item: ChecklistItem) => {
    if (item.status !== 'active' || event.target !== event.currentTarget) return;
    if (!picked || picked.id !== item.id) {
      if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); setPicked({ id: item.id, over: false }); setNotice('Карточка взята. Стрелка вправо переносит вперёд, Пробел отпускает, Esc отменяет.'); }
      return;
    }
    if (event.key === 'ArrowRight') { event.preventDefault(); setPicked({ id: item.id, over: true }); }
    else if (event.key === 'ArrowLeft') { event.preventDefault(); setPicked({ id: item.id, over: false }); }
    else if (event.key === 'Escape') { event.preventDefault(); setPicked(null); setNotice('Перенос отменён.'); }
    else if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); const over = picked.over; setPicked(null); drop(item, over ? 'review' : null); }
  };

  return (
    <section className="kanban" aria-label={`${trackName} чек-лист`}>
      <p id="kanban-help" className="visually-hidden">
        Активную карточку можно перетащить вперёд мышью или пальцем за ручку либо нажать кнопку на карточке. С клавиатуры: Пробел берёт карточку, стрелка вправо переносит, Пробел отпускает, Esc отменяет.
      </p>
      {error && <p className="notice error" role="alert">{error}</p>}
      {notice && <p className="kanban-notice" role="status">{notice}</p>}
      <div ref={columns} className={`kanban-columns${drag || picked ? ' is-dragging' : ''}`}>
        {KANBAN_COLUMNS.map(column => {
          const inColumn = cards.filter(card => columnOf(card.item) === column.status);
          const over = (drag?.over === column.status && DROP_TARGETS.includes(column.status)) || (picked?.over === true && column.status === 'review');
          return (
            <section key={column.status} data-status={column.status} className={`kanban-column is-${column.status}${over ? ' is-over' : ''}`} aria-label={column.title}>
              <header className="kanban-column-head">
                <h3>{column.title}</h3>
                <span>{inColumn.length}</span>
              </header>
              <div className="kanban-cards">
                {inColumn.length === 0 && <p className="kanban-empty">Пусто</p>}
                {inColumn.map(({ item, stage }) => {
                  const draggable = item.status === 'active' && busyID !== item.id;
                  const dragging = drag?.id === item.id;
                  const style = dragging ? { left: drag.left, top: drag.top, width: drag.width } : undefined;
                  return (
                    <div key={item.id} className="kanban-slot">
                      {dragging && <div className="kanban-card-ghost" style={{ height: drag.height }} aria-hidden="true" />}
                      <article
                        className={`kanban-card is-${column.status}${draggable ? ' is-draggable' : ''}${dragging ? ' is-dragging' : ''}${picked?.id === item.id ? ' is-picked' : ''}`}
                        style={style}
                        tabIndex={draggable ? 0 : undefined}
                        aria-describedby={draggable ? 'kanban-help' : undefined}
                        onPointerDown={event => begin(event, item)}
                        onClick={event => openCard(event, item)}
                        onKeyDown={event => onKey(event, item)}
                      >
                        <div className="kanban-card-top">
                          <span className="kanban-card-stage">
                            Недели {stage.week_start}{stage.week_end !== stage.week_start && `–${stage.week_end}`}
                          </span>
                          {draggable && <span className="kanban-handle" title="Потяните вперёд" aria-hidden="true">⠿</span>}
                        </div>
                        <h4>{item.title}</h4>
                        {item.description && <p>{item.description}</p>}
                        {item.estimated_minutes && <span className="kanban-card-time">≈ {item.estimated_minutes} мин</span>}
                        {busyID === item.id && <span className="kanban-card-time">Сохраняем…</span>}
                        {item.requires_review && <span className="kanban-card-time">Проверяет наставник</span>}
                        <div className="kanban-card-actions">
                          <button type="button" className="kanban-link" onClick={() => setOpenID(item.id)}>Подробнее</button>
                          {item.status === 'active' && (
                            <button type="button" className="kanban-next" disabled={busyID === item.id} onClick={() => void advance(item)}>{nextStepLabel(item)} →</button>
                          )}
                        </div>
                      </article>
                    </div>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>
      {opened && (
        <TaskModal
          item={opened.item}
          stage={opened.stage}
          checklistName={trackName}
          token={token}
          busy={busyID === opened.item.id}
          onClose={() => setOpenID('')}
          onAdvance={async result => {
            const item = opened.item;
            setBusyID(item.id);
            try {
              await advanceItem(token, item, result);
              setNotice(item.requires_review ? `«${item.title}» отправлена на проверку наставнику.` : '');
              await refresh();
            } finally { setBusyID(''); }
          }}
        />
      )}
    </section>
  );
}
