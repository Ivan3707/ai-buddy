export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request(baseUrl, path, { token, method = 'GET', body } = {}) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = await fetch(`${baseUrl}${path}`, {
    method, headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    if (response.status === 401 && typeof globalThis.dispatchEvent === 'function') {
      globalThis.dispatchEvent(new Event('ai-buddy:unauthorized'));
    }
    let message = `Backend: HTTP ${response.status}`;
    try {
      const payload = await response.json();
      if (typeof payload?.error === 'string') message = payload.error;
    } catch { /* The backend may return an empty or non-JSON error. */ }
    throw new ApiError(response.status, message);
  }
  if (response.status === 204) return undefined;
  return response.json();
}

export function chat(baseUrl, message, token) {
  return request(baseUrl, '/api/chat', { token, method: 'POST', body: { message } });
}
export function login(baseUrl, email, password) {
  return request(baseUrl, '/api/auth/login', { method: 'POST', body: { email, password } });
}
export function getMe(baseUrl, token) { return request(baseUrl, '/api/auth/me', { token }); }
export function createIntern(baseUrl, token, input) {
  return request(baseUrl, '/api/users', { token, method: 'POST', body: input });
}
export function getChecklists(baseUrl, token) { return request(baseUrl, '/api/checklists', { token }); }
export function completeItem(baseUrl, token, itemID) {
  return request(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/complete`, { token, method: 'POST' });
}
export function submitItem(baseUrl, token, itemID, input) {
  return request(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/submissions`, { token, method: 'POST', body: input });
}
export function getSubmissionHistory(baseUrl, token, itemID) {
  return request(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/submissions`, { token });
}
export function getPendingSubmissions(baseUrl, token) { return request(baseUrl, '/api/buddy/submissions', { token }); }
export function reviewSubmission(baseUrl, token, submissionID, input) {
  return request(baseUrl, `/api/buddy/submissions/${encodeURIComponent(submissionID)}/review`, { token, method: 'POST', body: input });
}
