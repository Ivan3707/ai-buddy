import { useState, type FormEvent } from 'react';
import type { User } from '@ai-buddy/client';
import { displayName } from '../lib/format';
import { sendFeedback } from '../lib/stubs';

export function FeedbackPage({ user }: { user: User }) {
  const [name, setName] = useState(displayName(user.email));
  const [email, setEmail] = useState(user.email);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState('');

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    setSent(false);
    try {
      await sendFeedback({ name: name.trim(), email: email.trim(), message });
      setSent(true);
      setMessage('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось отправить');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="feedback">
      <h1>Есть идеи? Мы открыты к вашим предложениям</h1>
      <p className="feedback-lead">Расскажи больше о себе и что у тебя на уме</p>
      <form onSubmit={submit}>
        <label>Имя<input value={name} onChange={event => setName(event.target.value)} /></label>
        <label>Почта<input type="email" value={email} onChange={event => setEmail(event.target.value)} /></label>
        <label>
          Опиши свою идею или задай вопрос
          <textarea required value={message} onChange={event => setMessage(event.target.value)} />
        </label>
        {error && <p className="feedback-note feedback-error" role="alert">{error}</p>}
        {sent && <p className="feedback-note" role="status">Спасибо! Сообщение отправлено.</p>}
        <button className="pill pill-black" disabled={busy}>{busy ? 'Отправляем…' : 'Отправить'}</button>
      </form>
    </main>
  );
}
