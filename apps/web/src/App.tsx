import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ApiError, getMe, logoutSession, refreshSession, type LoginResponse, type User } from '@ai-buddy/client';
import { LoginPage } from './components/LoginPage';
import { ChatPage, ChecklistsPage, ProfilePage, RequestsPage, TeamPage } from './components/Pages';
import { RolePlaceholder } from './components/RolePlaceholder';
import { CalendarPage, NotesPage } from './components/ProductivityPages';
import { DashboardPage } from './components/DashboardV2';
import { singleFlightRefresh } from './refresh';

type Session = { access: string; refresh: string; expiresAt: number };
const SESSION_KEY = 'ai-buddy-session-v2';
// Set to false to restore the existing buddy/teamlead Web interface.
const ROLE_PLACEHOLDER_ENABLED = true;
function readSession(): Session | null { try { const value = JSON.parse(sessionStorage.getItem(SESSION_KEY) || 'null') as Session | null; return value?.access && value.refresh ? value : null; } catch { return null; } }
const labels: Record<string, string> = { '/': 'Главная', '/checklists': 'Чек-листы', '/team': 'Команда', '/calendar': 'Календарь', '/notes': 'Заметки', '/requests': 'Заявки', '/chat': 'AI-помощник', '/profile': 'Профиль' };

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession);
  const [user, setUser] = useState<User | null>(null);
  const [path, setPath] = useState(location.pathname);
  const [loading, setLoading] = useState(Boolean(session)); const [error, setError] = useState(''); const [menu, setMenu] = useState(false);
  const sessionRef = useRef(session);
  const refreshFlightRef = useRef<Promise<Session> | null>(null);
  const loggingOutRef = useRef(false);
  const save = useCallback((next: Session | null) => { sessionRef.current = next; setSession(next); if (next) sessionStorage.setItem(SESSION_KEY, JSON.stringify(next)); else sessionStorage.removeItem(SESSION_KEY); }, []);
  const clear = useCallback(() => { save(null); setUser(null); setError(''); history.replaceState({}, '', '/'); setPath('/'); }, [save]);
  const refreshOnce = useMemo(() => singleFlightRefresh(async (current: Session) => {
    const result = await refreshSession('', current.refresh);
    if (sessionRef.current?.refresh !== current.refresh) throw new Error('Session changed during refresh');
    const next = { access: result.access_token, refresh: result.refresh_token, expiresAt: Date.now() + result.expires_in * 1000 };
    save(next);
    return next;
  }), [save]);
  const rotate = useCallback((current: Session) => {
    if (loggingOutRef.current) return Promise.reject(new Error('Session is closing'));
    if (sessionRef.current?.refresh !== current.refresh) return sessionRef.current ? Promise.resolve(sessionRef.current) : Promise.reject(new Error('Session ended'));
    const flight = refreshOnce(current);
    refreshFlightRef.current = flight;
    void flight.then(
      () => { if (refreshFlightRef.current === flight) refreshFlightRef.current = null; },
      () => { if (refreshFlightRef.current === flight) refreshFlightRef.current = null; },
    );
    return flight;
  }, [refreshOnce]);
  useEffect(() => { const pop = () => setPath(location.pathname); addEventListener('popstate', pop); return () => removeEventListener('popstate', pop); }, []);
  useEffect(() => { if (!session) { setLoading(false); return; } let cancelled = false; (async () => { setLoading(true); try { const active = session.expiresAt - Date.now() < 60_000 ? await rotate(session) : session; const me = await getMe('', active.access); if (!cancelled) setUser(me); } catch (reason) { if (!cancelled) { clear(); setError(reason instanceof Error ? reason.message : 'Не удалось восстановить сессию'); } } finally { if (!cancelled) setLoading(false); } })(); return () => { cancelled = true; }; }, []); // restore once
  useEffect(() => { if (!session) return; const delay = Math.max(1000, session.expiresAt - Date.now() - 60_000); const id = setTimeout(() => void rotate(session).catch(() => { if (sessionRef.current?.refresh === session.refresh) clear(); }), delay); return () => clearTimeout(id); }, [session, rotate, clear]);
  useEffect(() => { const unauthorized = (event: Event) => { const token = (event as CustomEvent<{ token: string }>).detail?.token; const current = sessionRef.current; if (current && token === current.access) void rotate(current).catch(() => { if (sessionRef.current?.refresh === current.refresh) clear(); }); }; addEventListener('ai-buddy:unauthorized', unauthorized); return () => removeEventListener('ai-buddy:unauthorized', unauthorized); }, [rotate, clear]);
  const login = (result: LoginResponse) => { loggingOutRef.current = false; const next = { access: result.access_token, refresh: result.refresh_token, expiresAt: Date.now() + result.expires_in * 1000 }; save(next); setUser(result.user); setError(''); };
  const logout = async () => { if (!session || loggingOutRef.current) return; loggingOutRef.current = true; let logoutError = ''; try { const active = await (refreshFlightRef.current ?? Promise.resolve(sessionRef.current ?? session)).catch(() => sessionRef.current ?? session); await logoutSession('', active.access, active.refresh); } catch (e) { if (!(e instanceof ApiError && e.status === 401)) logoutError = 'Сессия закрыта локально; сервер не подтвердил выход.'; } clear(); if (logoutError) setError(logoutError); };
  const nav = useMemo(() => { const common = ['/', '/calendar', '/notes', '/chat', '/profile']; if (user?.role === 'intern') return ['/', '/checklists', '/requests', ...common.slice(1)]; return ['/', '/team', ...common.slice(1)]; }, [user?.role]);
  const go = (next: string) => { history.pushState({}, '', next); setPath(next); setMenu(false); };
  if (loading) return <main className="center"><span className="loader"/><p>Восстанавливаем защищённую сессию…</p></main>;
  if (!user || !session) return <LoginPage error={error} onLogin={login}/>;
  if (ROLE_PLACEHOLDER_ENABLED && user.role !== 'intern') {
    return <RolePlaceholder user={user} onLogout={() => void logout()}/>;
  }
  if (!nav.includes(path)) queueMicrotask(() => go('/'));
  const page = path === '/checklists' && user.role === 'intern' ? <ChecklistsPage token={session.access}/> : path === '/team' && user.role !== 'intern' ? <TeamPage token={session.access} user={user}/> : path === '/calendar' ? <CalendarPage token={session.access}/> : path === '/notes' ? <NotesPage token={session.access}/> : path === '/requests' && user.role === 'intern' ? <RequestsPage token={session.access}/> : path === '/chat' ? <ChatPage token={session.access}/> : path === '/profile' ? <ProfilePage token={session.access}/> : <DashboardPage token={session.access} user={user}/>;
  return <div className="app-shell"><aside className={menu ? 'sidebar open' : 'sidebar'}><button className="brand-button" onClick={() => go('/')}>AI·BUDDY</button><nav>{nav.map(item => <button data-testid={`nav-${item === '/' ? 'home' : item.slice(1)}`} key={item} className={path === item ? 'active' : ''} onClick={() => go(item)}><span>{({ '/':'⌂','/checklists':'✓','/team':'◎','/calendar':'◫','/notes':'≡','/requests':'↗','/chat':'✦','/profile':'○' } as Record<string,string>)[item]}</span>{labels[item]}</button>)}</nav><div className="sidebar-foot"><div className="avatar">{(user.first_name?.[0] || user.email[0]).toUpperCase()}</div><div><b>{user.first_name || user.email}</b><small>{user.role}</small></div><button title="Выйти" onClick={() => void logout()}>↪</button></div></aside><section className="workspace"><header className="mobile-top"><button onClick={() => setMenu(!menu)}>☰</button><b>AI·BUDDY</b><span>{labels[path]}</span></header>{page}</section>{menu && <button className="scrim" aria-label="Закрыть меню" onClick={() => setMenu(false)}/>}</div>;
}
