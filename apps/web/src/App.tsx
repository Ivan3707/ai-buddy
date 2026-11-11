import { useEffect, useState } from 'react';
import { ApiError, getMe, type User } from '@ai-buddy/client';
import { LoginPage } from './components/LoginPage';
import { InternDashboard } from './components/InternDashboard';
import { BuddyDashboard } from './components/BuddyDashboard';

const TOKEN_KEY = 'ai-buddy-token';
export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem(TOKEN_KEY) ?? '');
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(Boolean(token)); const [error, setError] = useState('');
  const logout = () => { sessionStorage.removeItem(TOKEN_KEY); setToken(''); setUser(null); setError(''); };
  useEffect(() => {
    globalThis.addEventListener('ai-buddy:unauthorized', logout);
    return () => globalThis.removeEventListener('ai-buddy:unauthorized', logout);
  }, []);
  useEffect(() => {
    if (!token) { setLoading(false); return; }
    setLoading(true);
    getMe('', token).then(setUser).catch((reason: unknown) => {
      if (reason instanceof ApiError && reason.status === 401) logout();
      else setError(reason instanceof Error ? reason.message : 'Не удалось восстановить сессию');
    }).finally(() => setLoading(false));
  }, [token]);
  if (loading) return <main className="center"><div className="loader"/><p>Восстанавливаем сессию…</p></main>;
  if (!user) return <LoginPage error={error} onLogin={(nextToken, nextUser) => { sessionStorage.setItem(TOKEN_KEY, nextToken); setToken(nextToken); setUser(nextUser); setError(''); }}/>;
  return <div className="app-shell"><header className="topbar"><div><span className="eyebrow">AI-Buddy</span><strong>Онбординг без хаоса</strong></div><div className="account"><span>{user.email}</span><span className="role">{user.role === 'buddy' ? 'Buddy' : 'Стажёр'}</span><button className="ghost compact" onClick={logout}>Выйти</button></div></header>
    {user.role === 'intern' ? <InternDashboard token={token}/> : <BuddyDashboard token={token}/>}</div>;
}
