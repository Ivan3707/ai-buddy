export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export async function apiRequest(baseUrl, path, { token, method = 'GET', body } = {}) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = await fetch(`${baseUrl}${path}`, {
    method, headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    if (response.status === 401 && token && typeof globalThis.dispatchEvent === 'function') {
      globalThis.dispatchEvent(new CustomEvent('ai-buddy:unauthorized', { detail: { token } }));
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
  return apiRequest(baseUrl, '/api/chat', { token, method: 'POST', body: { message } });
}
export function login(baseUrl, email, password) {
  return apiRequest(baseUrl, '/api/auth/login', { method: 'POST', body: { email, password } });
}
export function refreshSession(baseUrl, refreshToken) { return apiRequest(baseUrl, '/api/auth/refresh', { method: 'POST', body: { refresh_token: refreshToken } }); }
export function logoutSession(baseUrl, token, refreshToken) { return apiRequest(baseUrl, '/api/auth/logout', { token, method: 'POST', body: { refresh_token: refreshToken } }); }
export function getMe(baseUrl, token) { return apiRequest(baseUrl, '/api/auth/me', { token }); }
export function getChecklists(baseUrl, token) { return apiRequest(baseUrl, '/api/checklists', { token }); }
export function getFeedback(baseUrl, token, checklistID) { return apiRequest(baseUrl, `/api/checklists/${encodeURIComponent(checklistID)}/feedback`, { token }); }
export function submitFeedback(baseUrl, token, checklistID, input) { return apiRequest(baseUrl, `/api/checklists/${encodeURIComponent(checklistID)}/feedback`, { token, method: 'POST', body: input }); }
export function completeItem(baseUrl, token, itemID) {
  return apiRequest(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/complete`, { token, method: 'POST' });
}
export function submitItem(baseUrl, token, itemID, input) {
  return apiRequest(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/submissions`, { token, method: 'POST', body: input });
}
export function getSubmissionHistory(baseUrl, token, itemID) {
  return apiRequest(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/submissions`, { token });
}
export function getPendingSubmissions(baseUrl, token) { return apiRequest(baseUrl, '/api/buddy/submissions', { token }); }
export function reviewSubmission(baseUrl, token, submissionID, input) {
  return apiRequest(baseUrl, `/api/buddy/submissions/${encodeURIComponent(submissionID)}/review`, { token, method: 'POST', body: input });
}
export function getProgress(baseUrl, token) { return apiRequest(baseUrl, '/api/progress', { token }); }
export function getAssignedInterns(baseUrl, token) { return apiRequest(baseUrl, '/api/staff/interns', { token }); }
export function getTeamleads(baseUrl, token) { return apiRequest(baseUrl, '/api/staff/teamleads', { token }); }
export function getBuddies(baseUrl, token) { return apiRequest(baseUrl, '/api/staff/buddies', { token }); }
export function getInternProgress(baseUrl, token, internID) { return apiRequest(baseUrl, `/api/staff/interns/${encodeURIComponent(internID)}/progress`, { token }); }
export function createPersonalTask(baseUrl, token, internID, input) { return apiRequest(baseUrl, `/api/teamlead/interns/${encodeURIComponent(internID)}/checklist-items`, { token, method: 'POST', body: input }); }
export function getCorporateProfile(baseUrl, token) { return apiRequest(baseUrl, '/api/corporate/me', { token }); }
export function listCorporateTools(baseUrl, token) { return apiRequest(baseUrl, '/api/corporate/tools', { token }); }
export function listAccessRequests(baseUrl, token) { return apiRequest(baseUrl, '/api/corporate/access-requests', { token }); }
export function createAccessRequest(baseUrl, token, input) { return apiRequest(baseUrl, '/api/corporate/access-requests', { token, method: 'POST', body: input }); }
export function listNotes(baseUrl, token) { return apiRequest(baseUrl, '/api/notes', { token }); }
export function createNote(baseUrl, token, input) { return apiRequest(baseUrl, '/api/notes', { token, method: 'POST', body: input }); }
export function updateNote(baseUrl, token, id, input) { return apiRequest(baseUrl, `/api/notes/${encodeURIComponent(id)}`, { token, method: 'PUT', body: input }); }
export function deleteNote(baseUrl, token, id) { return apiRequest(baseUrl, `/api/notes/${encodeURIComponent(id)}`, { token, method: 'DELETE' }); }
export function listEvents(baseUrl, token) { return apiRequest(baseUrl, '/api/calendar/events', { token }); }
export function createEvent(baseUrl, token, input) { return apiRequest(baseUrl, '/api/calendar/events', { token, method: 'POST', body: input }); }
export function updateEvent(baseUrl, token, id, input) { return apiRequest(baseUrl, `/api/calendar/events/${encodeURIComponent(id)}`, { token, method: 'PUT', body: input }); }
export function deleteEvent(baseUrl, token, id) { return apiRequest(baseUrl, `/api/calendar/events/${encodeURIComponent(id)}`, { token, method: 'DELETE' }); }
