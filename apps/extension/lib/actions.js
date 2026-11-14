// Presentation helpers for PendingAction cards (contracts/backend.openapi.json:
// PendingAction, ActionResult). The backend executes an action only after
// POST /api/actions/{id}/confirm, i.e. after an explicit click on "Подтвердить".

export const ACTION_TITLES = Object.freeze({
  leave_request: 'Заявка на отсутствие',
  access_request: 'Заявка на доступ',
  mentor_question: 'Вопрос наставнику',
});

const LEAVE_KINDS = {day_off: 'Отгул', vacation: 'Отпуск'};
const ACCESS_LEVELS = {read: 'чтение', write: 'запись', admin: 'администратор'};
const SYSTEMS = {
  jira: 'Jira', confluence: 'Confluence', gitlab: 'GitLab',
  grafana: 'Grafana', redash: 'Redash', kubernetes: 'Kubernetes',
};
const REQUEST_STATUSES = {
  pending_approval: 'на согласовании',
  approved: 'согласована',
  rejected: 'отклонена',
};

export function actionTitle(type) {
  return ACTION_TITLES[type] || 'Действие';
}

export function formatDate(value) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value || '');
  return match ? `${match[3]}.${match[2]}.${match[1]}` : String(value ?? '');
}

const text = value => (value === undefined || value === null || value === '' ? null : String(value));

// Returns [{label, value}] rows in display order; empty optional fields are skipped.
export function describeParams(type, params = {}) {
  let rows;
  switch (type) {
    case 'leave_request':
      rows = [
        ['Вид', LEAVE_KINDS[params.kind] || text(params.kind)],
        ['С', params.date_from ? formatDate(params.date_from) : null],
        ['По', params.date_to ? formatDate(params.date_to) : null],
        ['Комментарий', text(params.comment)],
      ];
      break;
    case 'access_request':
      rows = [
        ['Система', SYSTEMS[params.system] || text(params.system)],
        ['Ресурс', text(params.resource)],
        ['Уровень', ACCESS_LEVELS[params.access_level] || text(params.access_level)],
        ['Обоснование', text(params.justification)],
      ];
      break;
    case 'mentor_question':
      rows = [
        ['Вопрос', text(params.question)],
        ['Контекст', text(params.context)],
      ];
      break;
    default:
      rows = Object.entries(params).map(([key, value]) => [key, value !== null && typeof value === 'object' ? JSON.stringify(value) : text(value)]);
  }
  return rows.filter(([, value]) => value !== null).map(([label, value]) => ({label, value}));
}

// Text for an action that is no longer pending (from the stream or a confirm/cancel result).
export function describeActionStatus(action) {
  switch (action?.status) {
    case 'executed': return 'Выполнено.';
    case 'cancelled': return 'Отменено.';
    case 'failed': return action.error ? `Не удалось выполнить: ${action.error}` : 'Не удалось выполнить.';
    default: return '';
  }
}

export function describeResult(result) {
  const request = result?.request;
  if (request && typeof request.number === 'string') {
    const status = REQUEST_STATUSES[request.status] || request.status || 'создана';
    const approver = request.approver ? ` Согласующий: ${String(request.approver).replace(/\.$/, '')}.` : '';
    return {tone: 'success', text: `Заявка ${request.number} создана, статус: ${status}.${approver}`};
  }
  if (result?.mentor_question) {
    return {tone: 'success', text: 'Вопрос передан наставнику. Ответ появится в разделе вопросов.'};
  }
  const action = result?.action;
  if (action?.status === 'failed') return {tone: 'error', text: describeActionStatus(action)};
  if (action?.status === 'cancelled') return {tone: 'muted', text: 'Отменено.'};
  return {tone: 'success', text: 'Выполнено.'};
}

// HTTP error of confirm/cancel -> {text, retryable}. Retryable errors re-enable the buttons.
export function describeActionError(status, operation = 'confirm') {
  switch (status) {
    case 404: return {text: 'Действие не найдено или недоступно.', retryable: false};
    case 409: return operation === 'confirm'
      ? {text: 'Действие уже отменено.', retryable: false}
      : {text: 'Действие уже выполнено — отменить нельзя.', retryable: false};
    case 422: return {text: 'Параметры заявки некорректны. Уточните их в чате.', retryable: false};
    default: return {text: 'Не удалось связаться с backend. Попробуйте ещё раз.', retryable: true};
  }
}
