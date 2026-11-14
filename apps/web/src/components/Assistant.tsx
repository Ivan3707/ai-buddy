import { useState, type FormEvent } from 'react';
import { chat, type ChatResponse } from '@ai-buddy/client';

/** Плавающий AI-помощник в правом нижнем углу. */
export function Assistant({ token }: { token: string }) {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState('');
  const [reply, setReply] = useState<ChatResponse>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      setReply(await chat('', message.trim(), token));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Помощник недоступен');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="assistant" aria-label="AI-помощник">
      {open && (
        <div className="assistant-body">
          <form onSubmit={submit}>
            <label>
              Ваш вопрос
              <textarea
                maxLength={2000}
                required
                value={message}
                onChange={event => setMessage(event.target.value)}
                placeholder="Как оформить отпуск?"
              />
            </label>
            <button className="pill pill-dark" disabled={busy || !message.trim()}>{busy ? 'Ищем…' : 'Отправить'}</button>
          </form>
          {error && <p className="assistant-error" role="alert">{error}</p>}
          {reply && (
            <div className="assistant-answer">
              <p>{reply.answer}</p>
              {reply.sources.map(source => (
                <a key={source.id} href={source.url} target="_blank" rel="noreferrer">{source.title}</a>
              ))}
            </div>
          )}
        </div>
      )}
      <button
        className="assistant-toggle"
        aria-label="Спросить AI-помощника"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        ✦
      </button>
    </section>
  );
}
