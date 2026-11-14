/**
 * ЗАГЛУШКИ. Этих данных пока нет в API бэкенда (нет руководителя/наставника,
 * отдела, обратной связи). Когда Ваня добавит поля и методы — заменить здесь,
 * экраны менять не придётся.
 */
export const MANAGER_NAME = 'Юрий Пупупу';

/** Данные личного кабинета, которых нет в API (взяты из макета personal.png). */
export const PROFILE_STUB = {
  fullName: ['Кузнецов', 'Спайк', 'Максимович'],
  age: 25,
  photoUrl: null as string | null,
  manager: 'Новикова Буся Мариевна',
  mentor: 'Иванов Иван Иванович',
  department: 'Департамент цифровизации бизнес-процессов',
  unit: 'Отдел аналитики',
};

export type FeedbackInput = { name: string; email: string; message: string };

/** Имитация отправки обратной связи в корпоративный сервис (реального метода в API нет). */
export function sendFeedback(input: FeedbackInput): Promise<void> {
  if (!input.message.trim()) return Promise.reject(new Error('Напишите сообщение'));
  return new Promise(resolve => setTimeout(resolve, 400));
}
