import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, getMe, type User } from '@ai-buddy/client';
import { BuddyDashboard } from './components/BuddyDashboard';
import { LoginPage } from './components/LoginPage';
import { AppShell } from './components/shell/AppShell';
import { roleLabel } from './lib/format';

export const TOKEN_KEY = 'ai-buddy-token';

export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem(TOKEN_KEY) ?? '');
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(Boolean(token));
  const [error, setError] = useState('');
  // После входа пользователь уже известен — повторно запрашивать /me не нужно.
  const justLoggedIn = useRef(false);

  const logout = useCallback(() => {
    sessionStorage.removeItem(TOKEN_KEY);
    setToken('');
    setUser(null);
    setError('');
  }, []);

  useEffect(() => {
    globalThis.addEventListener('ai-buddy:unauthorized', logout);
    return () => globalThis.removeEventListener('ai-buddy:unauthorized', logout);
  }, [logout]);

  useEffect(() => {
    if (!token || justLoggedIn.current) {
      justLoggedIn.current = false;
      setLoading(false);
      return;
    }
    setLoading(true);
    getMe('', token)
      .then(setUser)
      .catch((reason: unknown) => {
        if (reason instanceof ApiError && reason.status === 401) logout();
        else setError(reason instanceof Error ? reason.message : 'Не удалось восстановить сессию');
      })
      .finally(() => setLoading(false));
  }, [token, logout]);

  if (loading) {
    return <main className="splash"><div className="loader" /><p>Восстанавливаем сессию…</p></main>;
  }

  if (!user) {
    return (
      <LoginPage
        error={error}
        onLogin={(nextToken, nextUser) => {
          sessionStorage.setItem(TOKEN_KEY, nextToken);
          justLoggedIn.current = true;
          setToken(nextToken);
          setUser(nextUser);
          setError('');
        }}
      />
    );
  }

  if (user.role === 'intern') return <AppShell user={user} token={token} onLogout={logout} />;

  // Кабинет buddy пока в прежнем оформлении: по макетам и по решению команды делаем только стажёра.
  return (
    <div className="legacy">
      <header className="topbar">
        <div><span className="eyebrow">AI-Buddy</span><strong>Онбординг без хаоса</strong></div>
        <div className="account">
          <span>{user.email}</span>
          <span className="role">{roleLabel(user.role)}</span>
          <button className="ghost compact" onClick={logout}>Выйти</button>
        </div>
      </header>
      <BuddyDashboard token={token} />
    </div>
  );
}
