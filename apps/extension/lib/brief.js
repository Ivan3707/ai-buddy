// Proactive brief (POST /api/chat/brief/stream): shown at most once per user per
// calendar day, and only when the panel opens without an ongoing conversation.

export const BRIEF_KEY_PREFIX = 'briefDate:';

export function todayKey(date = new Date()) {
  const pad = value => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export function briefStorageKey(userId) {
  return `${BRIEF_KEY_PREFIX}${userId}`;
}

export function shouldRequestBrief({userId, lastBriefDate, today, hasConversation}) {
  if (!userId || hasConversation) return false;
  return lastBriefDate !== today;
}
