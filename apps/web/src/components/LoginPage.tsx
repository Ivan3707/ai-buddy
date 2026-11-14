import { useState, type FormEvent } from 'react';
import { login, type LoginResponse } from '@ai-buddy/client';

export function LoginPage({ error: initialError, onLogin }: { error: string; onLogin: (result: LoginResponse) => void }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(initialError);
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('');
    try { onLogin(await login('', email.trim(), password)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось войти'); }
    finally { setBusy(false); }
  };
  return <main className="login-layout">
    <section className="login-intro"><span className="brand">AI·BUDDY</span><h1>Первые недели<br/>без лишнего шума.</h1><p>Чек-листы, встречи, заметки и поддержка команды в одном рабочем пространстве.</p><div className="login-art" aria-hidden="true"><i/><i/><i/></div></section>
    <section className="login-card"><div className="login-form"><span className="eyebrow">Рабочее пространство</span><h2>С возвращением</h2><p className="muted">Войдите с учётной записью стажёра, buddy или руководителя.</p><form onSubmit={submit}><label>Корпоративная почта<input data-testid="email" type="email" autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)} placeholder="name@company.ru"/></label><label>Пароль<input data-testid="password" type="password" autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)} placeholder="Ваш пароль"/></label>{error && <p className="notice error" role="alert">{error}</p>}<button data-testid="login" disabled={busy}>{busy ? 'Входим…' : 'Войти'}</button></form></div>
  </section></main>;
}
