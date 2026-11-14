import { useCallback, useEffect, useRef, useState } from 'react';
import { getChecklists, getMe, type User } from '@ai-buddy/client';
import '@buddy/buddy/widget.js';
import { createGateway } from '@buddy/lib/gateway.js';
import { auth } from '../session';
import { bot, ownQuestions, REQUEST_STATUS, type Hint } from './api';

// Бади on the site: the same widget as in the Chrome extension (apps/extension/buddy/widget.js), talking to the
// backend with the site session. If the extension is installed, its Бади takes over this page too (one character,
// one look) and the site only sends it the nudges below through window.postMessage.
// Nudges: a hint for a task the intern is stuck on (backend hint worker + AI /hint), a request status change,
// a mentor's answer; for staff — new escalated questions and requests.

type Nudge = { id: string; text: string; detail?: string; ask?: string; go?: string; hint?: Hint; happy?: boolean };

const read = (storage: Storage, key: string) => { try { const raw = storage.getItem(key); return raw === null ? undefined : JSON.parse(raw); } catch { return undefined; } };
const write = (storage: Storage, key: string, value: unknown) => { try { storage.setItem(key, JSON.stringify(value)); } catch { /* private mode */ } };
const extensionPresent = () => document.documentElement.dataset.aiBuddyExtension === '1';

export function BuddyHost({ token, user, onNavigate }: { token: string; user: User; onNavigate: (path: string) => void }) {
  const tokenRef = useRef(token);
  tokenRef.current = token;
  const [extension, setExtension] = useState(extensionPresent);
  const widget = useRef<BuddyApi | null>(null);
  const intern = user.role === 'intern';

  useEffect(() => {
    const on = () => setExtension(true);
    addEventListener('ai-buddy:extension', on);
    return () => removeEventListener('ai-buddy:extension', on);
  }, []);

  useEffect(() => {
    if (extension || !window.AIBuddyWidget) return;
    const prefix = `ai-buddy:${user.id}:`;
    const gateway = createGateway({
      baseUrl: '',
      auth,
      store: { // the conversation lives for the tab session, like the site session
        get: async key => read(sessionStorage, prefix + key),
        set: async (key, value) => write(sessionStorage, prefix + key, value),
        remove: async key => { try { sessionStorage.removeItem(prefix + key); } catch { /* ignore */ } },
      },
      client: { getMe },
    });
    const api = window.AIBuddyWidget.mount({
      call: gateway.call, stream: gateway.stream,
      storage: { // position, mode and window sizes: per browser
        get: async key => read(localStorage, `ai-buddy-widget:${key}`),
        set: async (key, value) => write(localStorage, `ai-buddy-widget:${key}`, value),
      },
      features: { login: false, pageContext: false, hide: false, requests: false },
      portalUrl: (kind: string, id: string) => `${location.origin}/portal/requests/${kind}/${encodeURIComponent(id)}`,
    });
    api.show();
    widget.current = api;
    return () => { api.remove(); widget.current = null; };
  }, [extension, user.id]);

  // ---------- nudges ----------
  const shown = useRef<Set<string>>(new Set());
  const pending = useRef<Map<string, Nudge>>(new Map());
  const titles = useRef<Map<string, string>>(new Map());

  const readHint = useCallback(async (n: Nudge) => {
    if (n.hint) { try { await bot.readHint(tokenRef.current, n.hint.id); } catch { /* ignore */ } }
  }, []);

  // The extension reports which nudge was handled ("Понятно" / "Обсудить").
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (event.source !== window || event.origin !== location.origin) return;
      const m = event.data as { source?: string; op?: string; id?: string };
      if (m?.source !== 'ai-buddy-extension' || m.op !== 'done' || !m.id) return;
      const n = pending.current.get(m.id);
      if (n) { pending.current.delete(m.id); void readHint(n); }
    };
    addEventListener('message', onMessage);
    return () => removeEventListener('message', onMessage);
  }, [readHint]);

  const push = useCallback((n: Nudge) => {
    if (shown.current.has(n.id)) return;
    shown.current.add(n.id);
    if (extension) {
      pending.current.set(n.id, n);
      window.postMessage({ source: 'ai-buddy-web', op: 'say', id: n.id, text: n.text, detail: n.detail, question: n.ask,
        mood: n.happy ? 'happy' : 'waving' }, location.origin);
      return;
    }
    const api = widget.current;
    if (!api) return;
    const done = () => void readHint(n);
    void api.say(n.text, {
      detail: n.detail, queue: true, ms: 0, mood: n.happy ? 'happy' : 'waving',
      onClick: () => { done(); if (n.go) onNavigate(n.go); else if (n.ask) void api.ask(n.ask); else void api.open(); },
      buttons: [
        ...(n.ask ? [{ label: 'Обсудить с Бади', onClick: () => { done(); void api.ask(n.ask!); } }] : []),
        ...(n.go ? [{ label: 'Открыть кабинет', onClick: () => { done(); onNavigate(n.go!); } }] : []),
        { label: 'Понятно', ghost: true, onClick: done },
      ],
    });
  }, [extension, onNavigate, readHint]);

  // Админ-панель: «Проверить» показывает пример всплывашки прямо сейчас. Id у каждого показа свой, повторы видны.
  useEffect(() => {
    const onPreview = (event: Event) => {
      const d = (event as CustomEvent<{ id?: unknown; text?: unknown; detail?: unknown; ask?: unknown }>).detail;
      if (!d || typeof d.id !== 'string' || typeof d.text !== 'string') return;
      push({ id: d.id, text: d.text, detail: typeof d.detail === 'string' ? d.detail : undefined, ask: typeof d.ask === 'string' ? d.ask : undefined });
    };
    window.addEventListener('ai-buddy:nudge-preview', onPreview);
    return () => window.removeEventListener('ai-buddy:nudge-preview', onPreview);
  }, [push]);

  const seenKey = `ai-buddy-seen:${user.id}`;
  const poll = useCallback(async () => {
    const token = tokenRef.current;
    const seen: Record<string, string> = read(localStorage, seenKey) || {};
    const first = !Object.keys(seen).length;
    try {
      if (intern) {
        const [hints, requests, questions] = await Promise.all([bot.hints(token).catch(() => []), bot.requests(token), ownQuestions(token)]);
        for (const h of hints.filter(h => !h.read_at)) {
          if (!titles.current.size) {
            const lists = await getChecklists('', token).catch(() => null);
            for (const c of lists ? [lists.common, lists.professional] : []) for (const st of c?.stages ?? []) for (const it of st.items) titles.current.set(it.id, it.title);
          }
          const title = titles.current.get(h.item_id);
          push({ id: `hint-${h.id}`, hint: h, detail: h.text, ask: title ? `Помоги с задачей «${title}»: с чего начать?` : undefined,
            text: title ? `Задача «${title}» идёт дольше плана. У меня есть подсказка:` : 'У меня есть подсказка по текущей задаче:' });
        }
        if (!extension) { // the extension's Бади watches request statuses itself
          for (const r of requests) {
            const key = `req-${r.id}`;
            if (!first && seen[key] && seen[key] !== r.status && r.status !== 'pending_approval')
              push({ id: `${key}-${r.status}`, happy: r.status === 'approved', text: `${r.status === 'approved' ? '🎉 ' : ''}Заявка ${r.number}: ${REQUEST_STATUS[r.status]}.` });
            seen[key] = r.status;
          }
        }
        for (const q of questions) {
          const key = `q-${q.id}`;
          if (!first && seen[key] === 'open' && q.status === 'answered')
            push({ id: `${key}-answered`, happy: true, text: 'Наставник ответил на твой вопрос — ответ уже в чате.' });
          seen[key] = q.status;
        }
      } else {
        const open = (await bot.questions(token, 'open')).length;
        const waiting = (await bot.staffRequests(token)).filter(r => r.request.status === 'pending_approval').length;
        const key = `staff-${open}-${waiting}`;
        if ((open || waiting) && seen.staff !== key)
          push({ id: key, go: '/mentor', text: `От бота: ${open ? `вопросов стажёров — ${open}` : ''}${open && waiting ? ', ' : ''}${waiting ? `заявок на согласование — ${waiting}` : ''}.` });
        seen.staff = key;
      }
      seen.init = '1';
      write(localStorage, seenKey, seen);
    } catch { /* offline or token refresh in progress: try next time */ }
  }, [extension, intern, push, seenKey]);

  useEffect(() => {
    const first = setTimeout(() => void poll(), 2500); // after the greeting
    const id = setInterval(() => void poll(), 20000);
    return () => { clearTimeout(first); clearInterval(id); };
  }, [poll]);

  return null;
}
