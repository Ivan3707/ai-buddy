import { useState, type FormEvent } from 'react';
import { login, type User } from '@ai-buddy/client';
import { LocationIcon } from './icons';

type Props = { error: string; onLogin: (token: string, user: User) => void };

export function LoginPage({ error: initialError, onLogin }: Props) {
  const [identity, setIdentity] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(initialError);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const result = await login('', identity.trim(), password);
      onLogin(result.access_token, result.user);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось войти');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="auth">
      <section className="auth-panel">
        <span className="lang">RU</span>
        <h1 className="auth-title">Вход</h1>
        <form className="auth-form" onSubmit={submit}>
          <label className="auth-field">
            Логин, почта, номер телефона
            <input
              type="text"
              autoComplete="username"
              required
              value={identity}
              onChange={event => setIdentity(event.target.value)}
            />
          </label>
          <label className="auth-field">
            Пароль
            <input
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={event => setPassword(event.target.value)}
            />
          </label>
          {error && <p className="auth-error" role="alert">{error}</p>}
          <button className="pill pill-light" disabled={busy}>{busy ? 'Входим…' : 'Войти'}</button>
        </form>
        <LocationIcon className="auth-pin" />
      </section>
      <section className="auth-art" aria-hidden="true">
        <span className="circle circle-green" />
        <span className="circle circle-blue" />
        <span className="circle circle-pink" />
      </section>
    </main>
  );
}
