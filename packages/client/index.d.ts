export type Role = 'intern' | 'buddy' | 'teamlead';
export type Profession = 'backend' | 'frontend' | 'ml';
export type ChecklistStatus = 'unavailable' | 'active' | 'completed' | 'verified';
export type SubmissionStatus = 'pending' | 'approved' | 'rejected';
export type PersonRef = { id: string; first_name: string | null; last_name: string | null; telegram_username: string | null };
export type User = { id: string; email: string; role: Role; profession: Profession | null; created_at: string; first_name: string | null; last_name: string | null; telegram_username: string | null; department: string | null; team: string | null; buddy?: PersonRef; teamlead?: PersonRef; age_years: number | null; tenure_months: number | null };
export type LoginResponse = { access_token: string; refresh_token: string; token_type: string; expires_in: number; is_first_login: boolean; user: User };
export type RefreshResponse = { access_token: string; refresh_token: string; token_type: string; expires_in: number };
export type ChecklistItem = { id: string; title: string; description: string; expected_result: string | null; requires_review: boolean; estimated_minutes: number | null; due_at: string | null; position: number; status: ChecklistStatus; completed_at: string | null;
  /** Backend always sends the next three fields (optional here only for older fixtures). The buddy returned the latest submission and the intern has not resubmitted: status is 'active' again ("Активно"). */
  returned_for_rework?: boolean;
  /** The buddy's comment of that return; null when not returned. */
  rework_comment?: string | null;
  /** Added for this intern by the teamlead (not from the shared catalog). */
  personal?: boolean };
export type ChecklistStage = { id: string; title: string; week_start: number; week_end: number; goal: string; expected_result: string; position: number; items: ChecklistItem[] };
export type Checklist = { id: string; name: string; type: 'common' | 'professional'; profession: Profession | null; stages: ChecklistStage[]; total_items: number; completed_items: number; completion_percent: number };
export type ChecklistsResponse = { common: Checklist; professional: Checklist };
export type Submission = { id: string; progress_id: string; attempt_number: number; result_text: string | null; result_url: string | null; status: SubmissionStatus; submitted_at: string };
export type SubmissionHistory = Submission & { reviewed_at: string | null; reviewed_by?: string | null; review_comment: string | null };
export type PendingSubmission = { id: string; intern_id: string; item_id: string; item_title: string; attempt_number: number; result_text: string | null; result_url: string | null; submitted_at: string };
export type ChatResponse = { answer: string; sources: { id: string; title: string; url: string }[]; mode: 'mock' };
/** Buddy and teamlead both receive common and professional; totals are over both checklists. */
export type ProgressResponse = { user_id: string; total_items: number; completed_items: number; completion_percent: number; common: Checklist; professional: Checklist };
export type ChecklistType = 'common' | 'professional';
/** Per-intern change of one task; omitted fields keep their value. */
export type InternTaskPatch = { title?: string; description?: string; expected_result?: string; estimated_minutes?: number; due_at?: string };
/** DEMO admin action result. */
export type DemoCompletion = { intern_id: string; items_completed: number; submissions_approved: number };
export type CorporateProfile = { id: string; email: string; role: Role; profession?: Profession; department: string; team: string; manager_id?: string; buddy_id?: string; allowed_tools: string[] };
export type AccessRequest = { id: string; tool: string; ticket_number: string; status: string; created_at: string };
export type Note = { id: string; title: string; content: string; updated_at: string };
export type CalendarEvent = { id: string; title: string; description: string; starts_at: string; ends_at: string };
export class ApiError extends Error { status: number; constructor(status: number, message: string); }
export function apiRequest<T = unknown>(baseUrl: string, path: string, options?: { token?: string; method?: string; body?: unknown }): Promise<T>;
export function chat(baseUrl: string, message: string, token?: string): Promise<ChatResponse>;
export function login(baseUrl: string, email: string, password: string): Promise<LoginResponse>;
export function refreshSession(baseUrl: string, refreshToken: string): Promise<RefreshResponse>;
export function linkSession(baseUrl: string, refreshToken: string): Promise<RefreshResponse>;
export function logoutSession(baseUrl: string, token: string, refreshToken: string): Promise<void>;
export function getMe(baseUrl: string, token: string): Promise<User>;
export function getChecklists(baseUrl: string, token: string): Promise<ChecklistsResponse>;
export type Feedback = { id: string; user_id: string; checklist_id: string; rating: number; comment: string; delivery_status: 'pending' | 'sent' | 'failed'; created_at: string; delivered_at: string | null };
export function getFeedback(baseUrl: string, token: string, checklistID: string): Promise<{feedback: Feedback | null}>;
export function submitFeedback(baseUrl: string, token: string, checklistID: string, input: {rating: number; comment: string}): Promise<Feedback>;
export function completeItem(baseUrl: string, token: string, itemID: string): Promise<void>;
export function submitItem(baseUrl: string, token: string, itemID: string, input: { result_text: string | null; result_url: string | null }): Promise<Submission>;
export function getSubmissionHistory(baseUrl: string, token: string, itemID: string): Promise<SubmissionHistory[]>;
export function getPendingSubmissions(baseUrl: string, token: string): Promise<PendingSubmission[]>;
export function reviewSubmission(baseUrl: string, token: string, submissionID: string, input: { approved: boolean; comment?: string }): Promise<void>;
export function getProgress(baseUrl: string, token: string): Promise<ProgressResponse>;
export function getAssignedInterns(baseUrl: string, token: string): Promise<User[]>;
export function getTeamleads(baseUrl: string, token: string): Promise<User[]>;
export function getBuddies(baseUrl: string, token: string): Promise<User[]>;
export function getInternProgress(baseUrl: string, token: string, internID: string): Promise<ProgressResponse>;
export function createPersonalTask(baseUrl: string, token: string, internID: string, input: { position: number; title: string; description: string; expected_result: string; estimated_minutes: number; requires_review: boolean; due_at?: string | null; checklist_type?: ChecklistType }): Promise<{ id: string }>;
/** Teamlead: change a task for this intern only (PATCH /api/teamlead/interns/{internID}/checklist-items/{itemID}). */
export function updateInternTask(baseUrl: string, token: string, internID: string, itemID: string, patch: InternTaskPatch): Promise<void>;
/** Teamlead: remove a task from this intern's checklist (soft delete, idempotent). */
export function removeInternTask(baseUrl: string, token: string, internID: string, itemID: string): Promise<void>;
/** Buddy or teamlead: review history of an intern's task (read-only). */
export function getInternSubmissionHistory(baseUrl: string, token: string, internID: string, itemID: string): Promise<SubmissionHistory[]>;
/** DEMO admin action: complete all checklists of the intern to 100%. */
export function adminCompleteChecklists(baseUrl: string, token: string, userID: string): Promise<DemoCompletion>;
export function getCorporateProfile(baseUrl: string, token: string): Promise<CorporateProfile>;
export function listCorporateTools(baseUrl: string, token: string): Promise<{tool: string; allowed: boolean}[]>;
export function listAccessRequests(baseUrl: string, token: string): Promise<AccessRequest[]>;
export function createAccessRequest(baseUrl: string, token: string, input: { tool: string; idempotency_key: string; confirm: true }): Promise<AccessRequest>;
export function listNotes(baseUrl: string, token: string): Promise<Note[]>;
export function createNote(baseUrl: string, token: string, input: { title: string; content: string }): Promise<Note>;
export function updateNote(baseUrl: string, token: string, id: string, input: { title: string; content: string }): Promise<Note>;
export function deleteNote(baseUrl: string, token: string, id: string): Promise<void>;
export function listEvents(baseUrl: string, token: string): Promise<CalendarEvent[]>;
export function createEvent(baseUrl: string, token: string, input: Omit<CalendarEvent, 'id'>): Promise<CalendarEvent>;
export function updateEvent(baseUrl: string, token: string, id: string, input: Omit<CalendarEvent, 'id'>): Promise<CalendarEvent>;
export function deleteEvent(baseUrl: string, token: string, id: string): Promise<void>;
export type TaskAttachment = { id: string; filename: string; content_type: string; size: number };
export type TaskComment = { id: string; author_id: string; author_name: string; author_role: 'intern' | 'buddy' | 'teamlead'; body: string; created_at: string; attachments: TaskAttachment[] };
export function listTaskComments(baseUrl: string, token: string, itemID: string, internID?: string): Promise<TaskComment[]>;
export function addTaskComment(baseUrl: string, token: string, itemID: string, input: { body?: string; files?: File[]; internId?: string }): Promise<TaskComment>;
export function fetchAttachment(baseUrl: string, token: string, attachmentID: string): Promise<Blob>;
