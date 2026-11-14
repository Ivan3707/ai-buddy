import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { type LoginResponse, type User } from '@ai-buddy/client';
import { LoginPage } from './components/LoginPage';
import { ChecklistsPage, ProfilePage, RequestsPage, TeamPage } from './components/Pages';
import { CalendarPage, NotesPage } from './components/ProductivityPages';
import { DashboardPage } from './components/DashboardV2';
import { BuddyInfoPage, MentorPage } from './bot/BuddyPages';
import { BuddyHost } from './bot/BuddyHost';
import { auth, currentUser, hasStoredSession } from './session';
import { extensionPresent, handshake, notifySignedOut } from './sso';

type Session = { access: string; expiresAt: number };
const labels: Record<string, string> = { '/': 'Главная', '/checklists': 'Чек-листы', '/team': 'Команда', '/calendar': 'Календарь', '/notes': 'Заметки', '/requests': 'Заявки', '/chat': 'Бади · AI', '/mentor': 'Кабинет наставника', '/profile': 'Профиль' };

export default function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const [path, setPath] = useState(location.pathname);
  const [loading, setLoading] = useState(hasStoredSession); const [error, setError] = useState(''); const [menu, setMenu] = useState(false);
  const userId = useRef('');
  userId.current = user?.id ?? '';
  // what the tokens in storage say -> React state (tokens are refreshed by lib/auth.js, never here)
  const sync = useCallback(async () => { const s = await auth.read(); setSession(s ? { access: s.access, expiresAt: s.expiresAt } : null); return s; }, []);
  const reset = useCallback((message = '') => { setSession(null); setUser(null); setError(message); history.replaceState({}, '', '/'); setPath('/'); }, []);
  // the session may belong to another user now (another tab / the extension): reload who it is
  const adopted = useCallback(async () => { const current = await currentUser(); if (!current) { reset(); return; } setUser(current.user); await sync(); setError(''); }, [reset, sync]);
  useEffect(() => { const pop = () => setPath(location.pathname); addEventListener('popstate', pop); return () => removeEventListener('popstate', pop); }, []);
  // restore once: a stored session survives closing the tab and the browser
  useEffect(() => { let cancelled = false; (async () => { try { const current = await currentUser(); if (cancelled) return; if (current) { setUser(current.user); await sync(); } } catch (reason) { if (!cancelled) setError(reason instanceof Error ? reason.message : 'Не удалось восстановить сессию'); } finally { if (!cancelled) setLoading(false); } })(); return () => { cancelled = true; }; }, []);
  // refresh shortly before the access token expires
  useEffect(() => { if (!session) return; const delay = Math.max(1000, session.expiresAt - Date.now() - 70_000); const id = setTimeout(async () => { const token = await auth.token({ force: true, stale: session.access }); if (!token) reset('Сессия завершена. Войдите снова.'); else await sync(); }, delay); return () => clearTimeout(id); }, [session, reset, sync]);
  // an API call got 401: refresh once (several callers at once share one refresh)
  useEffect(() => { const unauthorized = async () => { if (!session) return; const token = await auth.token({ force: true, stale: session.access }); if (!token) reset('Сессия завершена. Войдите снова.'); else await sync(); }; addEventListener('ai-buddy:unauthorized', unauthorized); return () => removeEventListener('ai-buddy:unauthorized', unauthorized); }, [session, reset, sync]);
  // another tab signed in / out / refreshed
  useEffect(() => auth.onChange(async () => { const s = await sync(); if (!s) { reset(); return; } if (!userId.current || s.sub !== userId.current) await adopted(); }), [sync, reset, adopted]);
  // single sign-on with the extension: on start, when the extension appears, and when it signs in / out
  useEffect(() => {
    const run = () => void handshake({ onSessionChanged: adopted, onSignedOut: async () => reset() });
    if (extensionPresent()) run();
    const onMessage = (event: MessageEvent) => { if (event.source === window && event.origin === location.origin && event.data?.source === 'ai-buddy-extension' && event.data.op === 'changed') run(); };
    addEventListener('ai-buddy:extension', run); addEventListener('message', onMessage);
    return () => { removeEventListener('ai-buddy:extension', run); removeEventListener('message', onMessage); };
  }, [adopted, reset]);
  const login = async (result: LoginResponse) => { await auth.adopt({ access_token: result.access_token, refresh_token: result.refresh_token, expires_in: result.expires_in }, { loginAt: Date.now() }); setUser(result.user); await sync(); setError(''); void handshake({ onSessionChanged: adopted, onSignedOut: async () => reset() }); };
  const logout = async () => { await auth.logout(); void notifySignedOut(); reset(); };
  const nav = useMemo(() => { const common = ['/', '/calendar', '/notes', '/chat', '/profile']; if (user?.role === 'intern') return ['/', '/checklists', '/requests', ...common.slice(1)]; return ['/', '/team', '/mentor', ...common.slice(1)]; }, [user?.role]);
  const go = (next: string) => { history.pushState({}, '', next); setPath(next); setMenu(false); };
  if (loading) return <main className="center"><span className="loader"/><p>Восстанавливаем защищённую сессию…</p></main>;
  if (!user || !session) return <LoginPage error={error} onLogin={login}/>;
  if (!nav.includes(path)) queueMicrotask(() => go('/'));
  const page = path === '/checklists' && user.role === 'intern' ? <ChecklistsPage token={session.access}/> : path === '/team' && user.role !== 'intern' ? <TeamPage token={session.access} user={user}/> : path === '/calendar' ? <CalendarPage token={session.access}/> : path === '/notes' ? <NotesPage token={session.access}/> : path === '/requests' && user.role === 'intern' ? <RequestsPage token={session.access}/> : path === '/chat' ? <BuddyInfoPage user={user}/> : path === '/mentor' && user.role !== 'intern' ? <MentorPage token={session.access}/> : path === '/profile' ? <ProfilePage token={session.access}/> : <DashboardPage token={session.access} user={user}/>;
  return <div className="app-shell"><aside className={menu ? 'sidebar open' : 'sidebar'}><button className="brand-button" onClick={() => go('/')}>AI·BUDDY</button><nav>{nav.map(item => <button data-testid={`nav-${item === '/' ? 'home' : item.slice(1)}`} key={item} className={path === item ? 'active' : ''} onClick={() => go(item)}><span>{({ '/':'⌂','/checklists':'✓','/team':'◎','/calendar':'◫','/notes':'≡','/requests':'↗','/chat':'✦','/mentor':'✉','/profile':'○' } as Record<string,string>)[item]}</span>{labels[item]}</button>)}</nav><div className="sidebar-foot"><div className="avatar">{(user.first_name?.[0] || user.email[0]).toUpperCase()}</div><div><b>{user.first_name || user.email}</b><small>{user.role}</small></div><button title="Выйти" onClick={() => void logout()}>↪</button></div></aside><section className="workspace"><header className="mobile-top"><button onClick={() => setMenu(!menu)}>☰</button><b>AI·BUDDY</b><span>{labels[path]}</span></header>{page}</section><BuddyHost token={session.access} user={user} onNavigate={go}/>{menu && <button className="scrim" aria-label="Закрыть меню" onClick={() => setMenu(false)}/>}</div>;
}
