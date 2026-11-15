export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export async function apiRequest(baseUrl, path, { token, method = 'GET', body, as = 'json' } = {}) {
  const headers = {};
  const isForm = typeof FormData !== 'undefined' && body instanceof FormData; // the browser sets the multipart boundary itself
  if (body !== undefined && !isForm) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = await fetch(`${baseUrl}${path}`, {
    method, headers,
    body: body === undefined ? undefined : isForm ? body : JSON.stringify(body),
    signal: AbortSignal.timeout(isForm || as === 'blob' ? 60000 : 15000),
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
  return as === 'blob' ? response.blob() : response.json();
}

export function chat(baseUrl, message, token) {
  return apiRequest(baseUrl, '/api/chat', { token, method: 'POST', body: { message } });
}
export function login(baseUrl, email, password) {
  return apiRequest(baseUrl, '/api/auth/login', { method: 'POST', body: { email, password } });
}
export function refreshSession(baseUrl, refreshToken) { return apiRequest(baseUrl, '/api/auth/refresh', { method: 'POST', body: { refresh_token: refreshToken } }); }
export function linkSession(baseUrl, refreshToken) { return apiRequest(baseUrl, '/api/auth/link', { method: 'POST', body: { refresh_token: refreshToken } }); }
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
export function updateInternTask(baseUrl, token, internID, itemID, patch) {
  return apiRequest(baseUrl, `/api/teamlead/interns/${encodeURIComponent(internID)}/checklist-items/${encodeURIComponent(itemID)}`, { token, method: 'PATCH', body: patch });
}
export function removeInternTask(baseUrl, token, internID, itemID) {
  return apiRequest(baseUrl, `/api/teamlead/interns/${encodeURIComponent(internID)}/checklist-items/${encodeURIComponent(itemID)}`, { token, method: 'DELETE' });
}
export function getInternSubmissionHistory(baseUrl, token, internID, itemID) {
  return apiRequest(baseUrl, `/api/staff/interns/${encodeURIComponent(internID)}/checklist-items/${encodeURIComponent(itemID)}/submissions`, { token });
}
export function adminCompleteChecklists(baseUrl, token, userID) {
  return apiRequest(baseUrl, `/api/admin/accounts/${encodeURIComponent(userID)}/complete-checklists`, { token, method: 'POST' });
}
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
export function listTaskComments(baseUrl, token, itemID, internID) {
  const query = internID ? `?intern_id=${encodeURIComponent(internID)}` : '';
  return apiRequest(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/comments${query}`, { token });
}
export function addTaskComment(baseUrl, token, itemID, { body = '', files = [], internId } = {}) {
  const form = new FormData();
  form.append('body', body);
  if (internId) form.append('intern_id', internId);
  for (const file of files) form.append('files', file, file.name);
  return apiRequest(baseUrl, `/api/checklist-items/${encodeURIComponent(itemID)}/comments`, { token, method: 'POST', body: form });
}
export function fetchAttachment(baseUrl, token, attachmentID) {
  return apiRequest(baseUrl, `/api/attachments/${encodeURIComponent(attachmentID)}`, { token, as: 'blob' });
}
