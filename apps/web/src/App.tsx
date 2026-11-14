import { useCallback, useEffect, useRef, useState } from 'react';
import { type LoginResponse, type User } from '@ai-buddy/client';
import { BuddyHost } from './bot/BuddyHost';
import { LoginPage } from './components/LoginPage';
import { AppShell } from './components/shell/AppShell';
import { StaffShell } from './components/staff/StaffShell';
import { PortalPage, parsePortalPath } from './pages/PortalPage';
import { auth, currentUser, hasStoredSession } from './session';
import { extensionPresent, handshake, notifySignedOut } from './sso';
import { markOnboardingPending } from './lib/onboarding';

type Session = { access: string; expiresAt: number };

/** Экран из подсказки Бади («есть вопросы стажёров»): оболочки слушают это событие и переключают вид. */
const navigate = (path: string) => window.dispatchEvent(new CustomEvent('ai-buddy:navigate', { detail: path.replace(/^\//, '') }));

export default function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(hasStoredSession);
  const [error, setError] = useState('');
  const userId = useRef('');
  userId.current = user?.id ?? '';

  // что говорят токены в хранилище -> состояние React (токены обновляет lib/auth.js, не этот файл)
  const sync = useCallback(async () => {
    const s = await auth.read();
    setSession(s ? { access: s.access, expiresAt: s.expiresAt } : null);
    return s;
  }, []);
  const reset = useCallback((message = '') => { setSession(null); setUser(null); setError(message); }, []);
  // сессия могла перейти к другому пользователю (другая вкладка, расширение): узнаём, кто это
  const adopted = useCallback(async () => {
    const current = await currentUser();
    if (!current) { reset(); return; }
    setUser(current.user);
    await sync();
    setError('');
  }, [reset, sync]);

  // восстановление сессии: вход переживает закрытие вкладки и браузера
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const current = await currentUser();
        if (cancelled) return;
        if (current) { setUser(current.user); await sync(); }
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : 'Не удалось восстановить сессию');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // обновляем токен заранее, до истечения
  useEffect(() => {
    if (!session) return;
    const delay = Math.max(1000, session.expiresAt - Date.now() - 70_000);
    const id = setTimeout(async () => {
      const token = await auth.token({ force: true, stale: session.access });
      if (!token) reset('Сессия завершена. Войдите снова.'); else await sync();
    }, delay);
    return () => clearTimeout(id);
  }, [session, reset, sync]);

  // запрос получил 401: один раз обновляем токен (несколько запросов делят одно обновление)
  useEffect(() => {
    const unauthorized = async () => {
      if (!session) return;
      const token = await auth.token({ force: true, stale: session.access });
      if (!token) reset('Сессия завершена. Войдите снова.'); else await sync();
    };
    addEventListener('ai-buddy:unauthorized', unauthorized);
    return () => removeEventListener('ai-buddy:unauthorized', unauthorized);
  }, [session, reset, sync]);

  // другая вкладка вошла, вышла или обновила токен
  useEffect(() => auth.onChange(async () => {
    const s = await sync();
    if (!s) { reset(); return; }
    if (!userId.current || s.sub !== userId.current) await adopted();
  }), [sync, reset, adopted]);

  // единый вход с расширением: при старте, когда расширение появилось и когда оно входит или выходит
  useEffect(() => {
    const run = () => void handshake({ onSessionChanged: adopted, onSignedOut: async () => reset() });
    if (extensionPresent()) run();
    const onMessage = (event: MessageEvent) => {
      if (event.source === window && event.origin === location.origin && event.data?.source === 'ai-buddy-extension' && event.data.op === 'changed') run();
    };
    addEventListener('ai-buddy:extension', run);
    addEventListener('message', onMessage);
    return () => { removeEventListener('ai-buddy:extension', run); removeEventListener('message', onMessage); };
  }, [adopted, reset]);

  const login = async (result: LoginResponse) => {
    if (result.is_first_login && result.user.role === 'intern') markOnboardingPending(result.user.id);
    await auth.adopt({ access_token: result.access_token, refresh_token: result.refresh_token, expires_in: result.expires_in }, { loginAt: Date.now() });
    setUser(result.user);
    await sync();
    setError('');
    void handshake({ onSessionChanged: adopted, onSignedOut: async () => reset() });
  };
  const logout = async () => { await auth.logout(); void notifySignedOut(); reset(); };

  if (loading) return <main className="splash"><div className="loader" /><p>Восстанавливаем сессию…</p></main>;
  if (!user || !session) return <LoginPage error={error} onLogin={login} />;

  // Псевдокорпоративный портал: по клику на заявку в окне Бади открывается её страница (адрес /portal/requests/...)
  const portal = parsePortalPath(location.pathname);
  if (portal) {
    return (
      <>
        <PortalPage user={user} token={session.access} route={portal} />
        <BuddyHost token={session.access} user={user} onNavigate={navigate} />
      </>
    );
  }

  // Один дизайн для всех: у стажёра и у наставника общие меню, сцена и боковые вкладки, различаются разделы.
  const Shell = user.role === 'intern' ? AppShell : StaffShell;
  return (
    <>
      <Shell user={user} token={session.access} onLogout={() => void logout()} />
      <BuddyHost token={session.access} user={user} onNavigate={navigate} />
    </>
  );
}
