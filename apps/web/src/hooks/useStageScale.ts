import { useEffect } from 'react';

/** Размер кадра макета Figma. */
const STAGE_WIDTH = 1920;
const STAGE_HEIGHT = 1080;
/** Ширина окна, до которой включается адаптивная раскладка вместо масштабируемого кадра. */
export const MOBILE_WIDTH = 900;

/** Масштаб кадра под окно: вписываем 1920×1080 целиком, чтобы блоки не ломались и не уходили за экран. */
export function stageScale(width: number, height: number): number {
  const scale = Math.min(width / STAGE_WIDTH, height / STAGE_HEIGHT);
  return Math.min(2.5, Math.max(0.5, scale));
}

/** Подгоняет интерфейс под разрешение монитора: масштабирует страницу пропорционально окну. */
export function useStageScale(): void {
  useEffect(() => {
    const root = document.documentElement;
    const apply = () => {
      // на телефонах и узких окнах (до 900 px) макет не масштабируется: там своя адаптивная раскладка (styles/responsive.css)
      const scale = window.innerWidth < MOBILE_WIDTH ? 1 : stageScale(window.innerWidth, window.innerHeight);
      root.style.zoom = String(scale);
      // vw/vh при zoom не пересчитываются — отдаём размеры окна в масштабированных пикселях
      root.style.setProperty('--stage-w', `${window.innerWidth / scale}px`);
      root.style.setProperty('--stage-h', `${window.innerHeight / scale}px`);
    };
    apply();
    window.addEventListener('resize', apply);
    return () => {
      window.removeEventListener('resize', apply);
      root.style.zoom = '';
      root.style.removeProperty('--stage-w');
      root.style.removeProperty('--stage-h');
    };
  }, []);
}
