/**
 * Онбординг показывается стажёру при первом входе: бэкенд отдаёт is_first_login при логине
 * (пока last_login_at пуст). Флаг «ещё не пройден» держим в localStorage, чтобы он
 * пережил перезагрузку страницы посреди тура. Для просмотра без нового пользователя
 * можно открыть приложение с ?onboarding.
 */
const keyFor = (userId: string) => `onboarding:${userId}`;

export function markOnboardingPending(userId: string): void {
  try { localStorage.setItem(keyFor(userId), 'pending'); } catch { /* без хранилища тур покажем один раз */ }
}

export function isOnboardingPending(userId: string): boolean {
  try {
    if (new URLSearchParams(globalThis.location.search).has('onboarding')) return true;
    return localStorage.getItem(keyFor(userId)) === 'pending';
  } catch {
    return false;
  }
}

export function finishOnboarding(userId: string): void {
  try { localStorage.removeItem(keyFor(userId)); } catch { /* нечего очищать */ }
}
