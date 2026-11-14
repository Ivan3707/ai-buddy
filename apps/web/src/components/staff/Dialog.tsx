import { useEffect, type ReactNode } from 'react';

type Props = { title: string; onClose: () => void; children: ReactNode };

/** Окно поверх страницы в оформлении сайта: кремовая карточка на затемнённом фоне, закрывается по Esc и по клику на фон. */
export function Dialog({ title, onClose, children }: Props) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <section className="dialog" role="dialog" aria-label={title} onClick={event => event.stopPropagation()}>
        <header className="dialog-head">
          <h2>{title}</h2>
          <button className="dialog-close" aria-label="Закрыть" onClick={onClose}>×</button>
        </header>
        <div className="dialog-body">{children}</div>
      </section>
    </div>
  );
}
