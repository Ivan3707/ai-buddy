import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { TOUR, WELCOME } from './content';

type Props = { onNavigate: (view: 'home' | 'tasks', checklist?: 'common' | 'professional') => void; onDone: () => void };
type Box = { top: number; left: number; width: number; height: number };

const ICONS: Record<string, string> = {
  tasks: 'M5 7l2 2 4-4M5 17l2 2 4-4M15 7h6M15 17h6',
  progress: 'M12 3a9 9 0 1 0 9 9M12 3v9h9',
  save: 'M20 11l-8.5 8.5a5 5 0 0 1-7-7L13 4a3.5 3.5 0 0 1 5 5l-8.5 8.5a2 2 0 0 1-3-3L14 7',
  growth: 'M4 19V9M10 19V5M16 19v-8M22 19H2',
};

function Icon({ name }: { name: string }) {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#252525" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={ICONS[name]} />
    </svg>
  );
}

function findTarget(selectors: string[]): Element | null {
  for (const selector of selectors) {
    const element = document.querySelector(selector);
    if (element) return element;
  }
  return null;
}

const pageZoom = () => Number.parseFloat(document.documentElement.style.zoom) || 1;

const GAP = 28;
const BOT_BUBBLE_ROOM = 150;
const PAD = 8;
const EDGE = 16;

/** Ставит подсказку справа, слева, снизу или сверху от элемента — где хватает места. */
export function placeBubble(box: Box, bubble: { width: number; height: number }, area: { width: number; height: number }) {
  const clampTop = (top: number) => Math.min(Math.max(EDGE, top), area.height - bubble.height - EDGE);
  const clampLeft = (left: number) => Math.min(Math.max(EDGE, left), area.width - bubble.width - EDGE);
  const centerTop = clampTop(box.top + box.height / 2 - bubble.height / 2);
  const centerLeft = clampLeft(box.left + box.width / 2 - bubble.width / 2);
  const right = box.left + box.width + GAP;
  if (right + bubble.width + EDGE <= area.width) return { top: centerTop, left: right };
  const left = box.left - GAP - bubble.width;
  if (left >= EDGE) return { top: centerTop, left };
  if (box.top + box.height + GAP + bubble.height + EDGE <= area.height) return { top: box.top + box.height + GAP, left: centerLeft };
  if (box.top - GAP - bubble.height >= EDGE) return { top: box.top - GAP - bubble.height, left: centerLeft };
  return { top: clampTop((area.height - bubble.height) / 2), left: clampLeft((area.width - bubble.width) / 2) };
}

function Welcome({ onStart, onSkip }: { onStart: () => void; onSkip: () => void }) {
  const startRef = useRef<HTMLButtonElement>(null);
  useEffect(() => startRef.current?.focus(), []);
  return (
    <div className="onb-backdrop">
      <section className="onb-welcome" role="dialog" aria-modal="true" aria-labelledby="onb-welcome-title">
        <header className="onb-welcome-head">
          <h2 id="onb-welcome-title">{WELCOME.title}</h2>
          <p>{WELCOME.lead}</p>
        </header>
        <div className="onb-welcome-body">
          <h3>{WELCOME.listTitle}</h3>
          <ul className="onb-features">
            {WELCOME.features.map(feature => (
              <li key={feature.title}>
                <span className="onb-feature-icon"><Icon name={feature.icon} /></span>
                <span><strong>{feature.title}</strong>{feature.text}</span>
              </li>
            ))}
          </ul>
          <div className="onb-actions">
            <button type="button" className="onb-skip" onClick={onSkip}>Пропустить</button>
            <button type="button" ref={startRef} className="pill-black onb-next" onClick={onStart}>Показать, как всё устроено</button>
          </div>
        </div>
      </section>
    </div>
  );
}

function Tour({ onNavigate, onDone }: Props) {
  const [index, setIndex] = useState(0);
  const [box, setBox] = useState<Box | null>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const bubbleRef = useRef<HTMLElement>(null);
  const nextRef = useRef<HTMLButtonElement>(null);
  const step = TOUR[index];
  const last = index === TOUR.length - 1;

  useEffect(() => { onNavigate(step.view, step.checklist); }, [step.view, step.checklist, onNavigate]);

  // Элемент может появиться чуть позже (данные ещё грузятся) — ищем несколько секунд
  const measure = useCallback(() => {
    if (step.corner) {
      // Бади нарисован в закрытом shadow root: измерить его нельзя, он стоит в углу окна (72 px, отступ 24) и не зависит от zoom страницы
      const zoom = pageZoom();
      const pad = 18; // запас шире рамки персонажа, чтобы кольцо не сливалось с его собственной обводкой
      const size = 72 + pad * 2;
      setBox({ top: (innerHeight - 96 - pad) / zoom, left: (innerWidth - 96 - pad) / zoom, width: size / zoom, height: size / zoom });
      return true;
    }
    const element = findTarget(step.targets);
    if (!element) { setBox(null); return false; }
    element.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    // При масштабе страницы (zoom на <html>) rect отдаётся в экранных пикселях, а положение оверлея задаётся в масштабированных
    const zoom = pageZoom();
    const rect = element.getBoundingClientRect();
    setBox({ top: rect.top / zoom - PAD, left: rect.left / zoom - PAD, width: rect.width / zoom + PAD * 2, height: rect.height / zoom + PAD * 2 });
    return true;
  }, [step]);

  useEffect(() => {
    let tries = 0;
    const timer = globalThis.setInterval(() => {
      tries += 1;
      if (measure() || tries > 30) globalThis.clearInterval(timer);
    }, 100);
    const onResize = () => { measure(); };
    globalThis.addEventListener('resize', onResize);
    return () => { globalThis.clearInterval(timer); globalThis.removeEventListener('resize', onResize); };
  }, [measure]);

  useLayoutEffect(() => {
    const bubble = bubbleRef.current;
    if (!bubble) return;
    const zoom = pageZoom();
    const area = { width: document.documentElement.clientWidth / zoom, height: document.documentElement.clientHeight / zoom };
    const size = { width: bubble.offsetWidth, height: bubble.offsetHeight };
    if (step.corner && box) {
      // над персонажем: ниже стоит его собственная всплывашка («Привет! У меня есть сводка…»), она рисуется поверх всего
      setPos({ top: Math.max(EDGE, box.top - BOT_BUBBLE_ROOM - size.height), left: Math.max(EDGE, area.width - size.width - EDGE) });
      return;
    }
    setPos(box ? placeBubble(box, size, area) : placeBubble({ top: area.height / 2, left: area.width / 2, width: 0, height: 0 }, size, area));
  }, [box, index, step.corner]);

  useEffect(() => { nextRef.current?.focus(); }, [index]);

  const next = useCallback(() => (last ? onDone() : setIndex(value => value + 1)), [last, onDone]);
  const back = useCallback(() => setIndex(value => Math.max(0, value - 1)), []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onDone();
      else if (event.key === 'ArrowRight') next();
      else if (event.key === 'ArrowLeft') back();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [next, back, onDone]);

  return (
    <div className="onb-tour">
      {box ? <div className="onb-spotlight" style={box} aria-hidden="true" /> : <div className="onb-dim" aria-hidden="true" />}
      <section
        ref={bubbleRef}
        className="onb-bubble"
        role="dialog"
        aria-modal="true"
        aria-labelledby="onb-step-title"
        style={pos}
      >
        <span className="onb-counter">{index + 1} из {TOUR.length}</span>
        <h3 id="onb-step-title">{step.title}</h3>
        {step.text.map(paragraph => <p key={paragraph}>{paragraph}</p>)}
        <div className="onb-dots" aria-hidden="true">
          {TOUR.map((item, dot) => <span key={item.title} className={dot === index ? 'is-current' : dot < index ? 'is-past' : ''} />)}
        </div>
        <div className="onb-actions">
          {last ? <span /> : <button type="button" className="onb-skip" onClick={onDone}>Пропустить</button>}
          <span className="onb-nav">
            {index > 0 && <button type="button" className="onb-back" onClick={back}>Назад</button>}
            <button type="button" ref={nextRef} className="pill-black onb-next" onClick={next}>{last ? 'Начать' : 'Далее'}</button>
          </span>
        </div>
      </section>
    </div>
  );
}

/** Приветствие и тур по интерфейсу для стажёра, который зашёл впервые. */
export function Onboarding({ onNavigate, onDone }: Props) {
  const [started, setStarted] = useState(false);
  return started ? <Tour onNavigate={onNavigate} onDone={onDone} /> : <Welcome onStart={() => setStarted(true)} onSkip={onDone} />;
}
