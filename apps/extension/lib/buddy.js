// Pure logic of the on-page character ("Бади"): page hints, request status notifications,
// view model for the content script, drag clamping and per-site match patterns.
// The content script is a dumb view: everything here runs in the background worker or tests.
import { actionTitle, describeActionStatus, describeParams } from './actions.js';
import { statusLabel } from './stream-events.js';

export const REQUEST_STATUS_TEXT = Object.freeze({
  pending_approval: 'на согласовании',
  approved: 'согласована',
  rejected: 'отклонена',
  done: 'выполнена',
});

// Proactive hint for the page the character is on. Page text is untrusted: only the title/url are
// matched, and the question sent later is ours, not the page's.
const HINTS = [
  {test: /grafana|дашборд|dashboard/i, text: 'Это дашборды метрик. Подсказать, что смотреть в первую очередь?',
    question: 'Что тут смотреть в первую очередь?'},
  {test: /jira|тикет|issue|board/i, text: 'Помочь взять задачу в Jira и оформить её по правилам?',
    question: 'Как здесь правильно взять задачу в работу?'},
  {test: /merge.?request|pull.?request|gitlab|github|code.?review|ревью/i, text: 'Напомнить правила код-ревью и мержа?',
    question: 'Какие правила ревью и мержа тут действуют?'},
  {test: /wiki|confluence|вики|регламент|faq|памятк/i, text: 'Выдели текст — проверю, актуален ли он по базе знаний.',
    question: null},
];

export function pageHint({url = '', title = ''} = {}) {
  const haystack = `${title} ${url}`;
  const hint = HINTS.find(item => item.test.test(haystack));
  return hint ? {text: hint.text, question: hint.question} : null;
}

// Compares the user's requests with the last seen statuses.
// Returns {changes: [{number, text}], next}; the first call only remembers (no flood of old news).
export function requestStatusChanges(previous, requests) {
  const next = {};
  const changes = [];
  for (const request of Array.isArray(requests) ? requests : []) {
    if (!request || typeof request.number !== 'string' || typeof request.status !== 'string') continue;
    next[request.number] = request.status;
    const before = previous?.[request.number];
    if (previous && before && before !== request.status) {
      const status = REQUEST_STATUS_TEXT[request.status] || request.status;
      changes.push({number: request.number, status: request.status, text: `Заявка ${request.number}: ${status}`});
    }
  }
  return {changes, next};
}

// Character mood for the current answer: drives the CSS animation of the mascot.
export function moodOf(state) {
  if (!state) return 'idle';
  if (state.error) return 'sad';
  if (!state.finished) return state.text ? 'talking' : 'thinking';
  if (state.action?.status === 'pending') return 'waiting';
  return 'idle';
}

// Serializable view model posted to the content script (no functions, only display strings).
export function toView(state) {
  const action = state.action;
  return {
    text: state.text,
    status: statusLabel(state),
    mood: moodOf(state),
    finished: state.finished,
    messageId: state.finished && !state.error ? state.messageId : null,
    error: state.error ? state.error.message : null,
    form: state.form,
    sources: state.sources
      .filter(source => /^https?:\/\//.test(source.url)) // no javascript:/data: links in the page
      .map(source => ({id: source.id, title: source.title, url: source.url, snippet: source.snippet || '', ...(source.refs ? {refs: source.refs} : {})})),
    card: action ? {
      id: action.id,
      title: actionTitle(action.type),
      summary: action.summary,
      rows: describeParams(action.type, action.params),
      pending: action.status === 'pending',
      result: action.status === 'pending' ? '' : describeActionStatus(action),
    } : null,
  };
}

// Keeps the character fully on screen after a drag or a window resize.
export function clampPosition({x, y}, {width, height}, size, margin = 8) {
  const fix = value => (Number.isFinite(value) ? value : Infinity);
  return {
    x: Math.round(Math.min(Math.max(fix(x), margin), Math.max(margin, width - size - margin))),
    y: Math.round(Math.min(Math.max(fix(y), margin), Math.max(margin, height - size - margin))),
  };
}

// "https://wiki.example.com/page?x" -> "https://wiki.example.com/*" (only http/https pages).
export function siteMatchPattern(raw) {
  let url;
  try { url = new URL(raw); } catch { return null; }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return null;
  return `${url.protocol}//${url.hostname}/*`;
}

export function displayName(user) {
  if (typeof user?.display_name === 'string' && user.display_name.trim()) return user.display_name.trim();
  if (typeof user?.first_name === 'string' && user.first_name.trim()) return user.first_name.trim();
  const local = typeof user?.email === 'string' ? user.email.split('@')[0] : '';
  return /^[a-zа-яё]+$/i.test(local) && !/^(intern|user|test|demo|buddy|teamlead|admin)$/i.test(local) ? local : '';
}
