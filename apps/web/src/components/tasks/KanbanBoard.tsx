import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react';
import { completeItem, type Checklist, type ChecklistItem } from '@ai-buddy/client';
import { History, ReviewAction } from './TaskReview';

type Status = ChecklistItem['status'];

/** Колонки доски: порядок и подписи заданы дизайнером. */
export const KANBAN_COLUMNS: { status: Status; title: string }[] = [
  { status: 'unavailable', title: 'Недоступно' },
  { status: 'active', title: 'Активно' },
  { status: 'completed', title: 'Завершено' },
  { status: 'verified', title: 'Проверено' },
];

/** Единственный переход, который делает стажёр сам: активная задача -> «Завершено». Остальное — решение бэкенда и наставника. */
const DROP_STATUS: Status = 'completed';
const DRAG_DISTANCE = 5;
const EDGE_ZONE = 56;

type Props = { checklist: Checklist; token: string; refresh: () => Promise<void> };
type Drag = { id: string; left: number; top: number; width: number; height: number; over: Status | null };
type Gesture = { id: string; startX: number; startY: number; offsetX: number; offsetY: number; rect: DOMRect; active: boolean; pointerId: number };

const INTERACTIVE = 'button, a, input, textarea, select, label, form, summary, .history';

/**
 * Задачи одного чек-листа в виде канбан-доски по статусам.
 * Активную задачу (без проверки наставником) перетаскивают в «Завершено»: мышью, пальцем за ручку ⠿ или с клавиатуры.
 * Задачи с проверкой завершаются отправкой результата в форме на карточке: перенос в «Завершено» подсказывает об этом.
 */
export function KanbanBoard({ checklist, token, refresh }: Props) {
  const [busyID, setBusyID] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [drag, setDrag] = useState<Drag | null>(null);
  const [picked, setPicked] = useState<{ id: string; over: boolean } | null>(null);
  const [flashID, setFlashID] = useState('');
  const gesture = useRef<Gesture | null>(null);
  const pointer = useRef({ x: 0, y: 0 });
  const frame = useRef(0);
  const columns = useRef<HTMLDivElement>(null);
  const trackName = checklist.type === 'common' ? 'Общий' : 'Профессиональный';
  const cards = checklist.stages.flatMap(stage => stage.items.map(item => ({ item, stage })));
  const byID = useCallback((id: string) => cards.find(card => card.item.id === id)?.item, [cards]);

  const complete = async (item: ChecklistItem) => {
    setBusyID(item.id);
    setError('');
    setNotice('');
    try {
      await completeItem('', token, item.id);
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
    if (target === null || target === item.status) { setNotice(''); return; }
    if (target !== DROP_STATUS) {
      setNotice('Карточку можно перенести только в «Завершено». «Проверено» ставит наставник.');
      return;
    }
    if (item.requires_review) {
      setNotice(`«${item.title}» принимает наставник: опишите результат в форме на карточке и отправьте на проверку.`);
      setFlashID(item.id);
      setTimeout(() => setFlashID(''), 1600);
      return;
    }
    setNotice('');
    void complete(item);
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
    if (wasDragging && item) drop(item, target);
  }
  function onCancel(event: PointerEvent) {
    if (gesture.current && event.pointerId === gesture.current.pointerId) stop();
  }

  useEffect(() => stop, []); // eslint-disable-line react-hooks/exhaustive-deps

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

  // Клавиатура: Пробел или Enter берёт карточку, стрелка вправо переносит в «Завершено», Пробел отпускает, Esc отменяет.
  const onKey = (event: KeyboardEvent<HTMLElement>, item: ChecklistItem) => {
    if (item.status !== 'active' || event.target !== event.currentTarget) return;
    if (!picked || picked.id !== item.id) {
      if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); setPicked({ id: item.id, over: false }); setNotice('Карточка взята. Стрелка вправо переносит в «Завершено», Пробел отпускает, Esc отменяет.'); }
      return;
    }
    if (event.key === 'ArrowRight') { event.preventDefault(); setPicked({ id: item.id, over: true }); }
    else if (event.key === 'ArrowLeft') { event.preventDefault(); setPicked({ id: item.id, over: false }); }
    else if (event.key === 'Escape') { event.preventDefault(); setPicked(null); setNotice('Перенос отменён.'); }
    else if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); const over = picked.over; setPicked(null); drop(item, over ? DROP_STATUS : null); }
  };

  return (
    <section className="kanban" aria-label={`${trackName} чек-лист`}>
      <p id="kanban-help" className="visually-hidden">
        Активную карточку можно перетащить в колонку «Завершено» мышью или пальцем за ручку. С клавиатуры: Пробел берёт карточку, стрелка вправо переносит, Пробел отпускает, Esc отменяет.
      </p>
      {error && <p className="notice error" role="alert">{error}</p>}
      {notice && <p className="kanban-notice" role="status">{notice}</p>}
      <div ref={columns} className={`kanban-columns${drag || picked ? ' is-dragging' : ''}`}>
        {KANBAN_COLUMNS.map(column => {
          const inColumn = cards.filter(card => card.item.status === column.status);
          const over = (drag?.over === column.status && column.status === DROP_STATUS) || (picked?.over === true && column.status === DROP_STATUS);
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
                        className={`kanban-card is-${item.status}${draggable ? ' is-draggable' : ''}${dragging ? ' is-dragging' : ''}${picked?.id === item.id ? ' is-picked' : ''}${flashID === item.id ? ' is-flash' : ''}`}
                        style={style}
                        tabIndex={draggable ? 0 : undefined}
                        aria-describedby={draggable ? 'kanban-help' : undefined}
                        onPointerDown={event => begin(event, item)}
                        onKeyDown={event => onKey(event, item)}
                      >
                        <div className="kanban-card-top">
                          <span className="kanban-card-stage">
                            Недели {stage.week_start}{stage.week_end !== stage.week_start && `–${stage.week_end}`}
                          </span>
                          {draggable && <span className="kanban-handle" title="Потяните в «Завершено»" aria-hidden="true">⠿</span>}
                        </div>
                        <h4>{item.title}</h4>
                        {item.description && <p>{item.description}</p>}
                        {item.estimated_minutes && <span className="kanban-card-time">≈ {item.estimated_minutes} мин</span>}
                        {busyID === item.id && <span className="kanban-card-time">Сохраняем…</span>}
                        {item.status === 'active' && item.requires_review && <ReviewAction item={item} token={token} refresh={refresh} />}
                        {item.requires_review && <History itemID={item.id} token={token} />}
                      </article>
                    </div>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>
    </section>
  );
}
