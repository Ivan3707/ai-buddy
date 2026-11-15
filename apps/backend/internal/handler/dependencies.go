package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type AuthService interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
}

type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

type TeamReader interface {
	ListAssignedInterns(context.Context, uuid.UUID, model.UserRole) ([]model.User, error)
	ListTeamleads(context.Context) ([]model.User, error)
	ListBuddies(context.Context) ([]model.User, error)
}

type ChecklistReader interface {
	GetByUserID(context.Context, uuid.UUID) (*service.UserChecklists, error)
}

type ChecklistItemService interface {
	CompleteItem(context.Context, uuid.UUID, uuid.UUID) error
	SubmitItem(context.Context, uuid.UUID, uuid.UUID, *string, *string) (model.ChecklistItemSubmission, error)
	ListSubmissions(context.Context, uuid.UUID, uuid.UUID) ([]model.ChecklistItemSubmission, error)
}

type BuddySubmissionService interface {
	ListPendingSubmissions(context.Context, uuid.UUID) ([]service.PendingSubmission, error)
	ReviewSubmission(context.Context, uuid.UUID, uuid.UUID, bool, *string) error
}

// PersonalTaskCreator adds a personal task to the common or professional checklist of the teamlead's intern.
type PersonalTaskCreator interface {
	CreateInChecklist(context.Context, uuid.UUID, uuid.UUID, model.ChecklistType, int, string, string, string, int, bool, *time.Time) (uuid.UUID, error)
}

// InternChecklistEditor changes or removes one task of one intern (teamlead only; per-intern, the catalog is not changed).
type InternChecklistEditor interface {
	UpdateTask(ctx context.Context, teamleadID, internID, itemID uuid.UUID, edit service.TaskEdit) error
	RemoveTask(ctx context.Context, teamleadID, internID, itemID uuid.UUID) error
}

// StaffSubmissionReader is the read-only review history of an intern's task for their buddy or teamlead.
type StaffSubmissionReader interface {
	ListInternSubmissions(ctx context.Context, actor *model.User, internID, itemID uuid.UUID) ([]model.ChecklistItemSubmission, error)
}

type HintReader interface {
	List(context.Context, uuid.UUID) ([]repository.Hint, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (*repository.Hint, error)
	Read(context.Context, uuid.UUID, uuid.UUID) error
}

type SessionChecker interface {
	IsActive(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

type SessionService interface {
	Refresh(context.Context, string) (string, string, int64, error)
	Link(context.Context, string) (string, string, int64, error)
	Logout(context.Context, uuid.UUID, string) error
}

// ---------------------------------------------------------------- assistant, admin and task comments (ARCH-06)
//
// Narrow interfaces satisfied by *service.AssistantService, *service.AdminService and *service.TaskCommentService.
// Handlers depend on them only, so a fake can replace the service in handler tests.

// AssistantChat is the streaming chat, its history and answer feedback.
type AssistantChat interface {
	UserContext(context.Context, *model.User) (service.UserContext, error)
	StartTurn(context.Context, uuid.UUID, *uuid.UUID, string) (uuid.UUID, []service.ChatTurn, error)
	BriefConversation(context.Context, uuid.UUID) (uuid.UUID, error)
	Relay(context.Context, uuid.UUID, uuid.UUID, string, any, service.Emit) error
	Messages(context.Context, uuid.UUID, *uuid.UUID) (*uuid.UUID, []service.ChatMessage, error)
	Feedback(context.Context, uuid.UUID, uuid.UUID, string, string) error
}

// AssistantActions confirms or cancels actions proposed in the chat and creates them from forms.
type AssistantActions interface {
	Confirm(context.Context, *model.User, uuid.UUID) (service.ActionResult, error)
	Cancel(context.Context, uuid.UUID, uuid.UUID) (service.PendingAction, error)
	CreateRequest(context.Context, *model.User, string, json.RawMessage) (service.ActionResult, error)
	CreateQuestion(context.Context, *model.User, string) (service.MentorQuestion, error)
}

// AssistantRequests is the mock corporate request service (own requests and teamlead approval).
type AssistantRequests interface {
	ListRequests(context.Context, uuid.UUID, int) ([]service.ServiceRequest, error)
	GetRequest(context.Context, *model.User, uuid.UUID) (service.ServiceRequest, error)
	DecideRequest(context.Context, *model.User, uuid.UUID, string, string) (service.ServiceRequest, error)
	StaffRequests(context.Context, *model.User) ([]map[string]any, error)
}

// AssistantQuestions is the mentor side: escalated questions, answers and AI drafts.
type AssistantQuestions interface {
	Questions(context.Context, *model.User, string) ([]service.MentorQuestion, error)
	Answer(context.Context, *model.User, uuid.UUID, string) (service.MentorQuestion, error)
	OwnQuestions(context.Context, *model.User) ([]service.MentorQuestion, error)
	Draft(context.Context, *model.User, uuid.UUID) (json.RawMessage, error)
}

// AssistantKnowledge is the knowledge base reader, ready-made suggestions and chat metrics.
type AssistantKnowledge interface {
	Suggestions(context.Context) (json.RawMessage, error)
	HintTurn(context.Context, uuid.UUID, *uuid.UUID, string) (service.HintTurnResult, error)
	KBDoc(context.Context, string) (json.RawMessage, error)
	Metrics(context.Context, *model.User) (service.ChatMetrics, error)
}

// Assistant is everything the assistant routes need.
type Assistant interface {
	AssistantChat
	AssistantActions
	AssistantRequests
	AssistantQuestions
	AssistantKnowledge
}

// Admin is what the admin panel routes need.
type Admin interface {
	IsOpenForAll() bool
	AnswerFeedback(context.Context) (service.AnswerFeedback, error)
	RestartOnboarding(context.Context, string) error
	Accounts(context.Context) ([]service.AdminAccount, error)
	KBLint(context.Context) (json.RawMessage, error)
	KBGaps(context.Context) (json.RawMessage, error)
	LLM(context.Context) (json.RawMessage, error)
	LLMPing(context.Context) (json.RawMessage, error)
	ReloadKB(context.Context) (json.RawMessage, error)
	System(context.Context) service.AdminSystem
	Notifications(context.Context) []service.NotificationInfo
	CheckStuckTask(context.Context) (service.StuckCheck, error)
	CompleteChecklists(ctx context.Context, actorID, internID uuid.UUID) (service.DemoCompletion, error)
}

// TaskComments is what the task comment routes need.
type TaskComments interface {
	List(context.Context, *model.User, uuid.UUID, *uuid.UUID) ([]service.TaskComment, error)
	Add(context.Context, *model.User, uuid.UUID, *uuid.UUID, string, []service.CommentFile) (*service.TaskComment, error)
	Attachment(context.Context, *model.User, uuid.UUID) (*service.AttachmentFile, error)
}

var (
	_ PersonalTaskCreator   = (*service.PersonalTaskService)(nil)
	_ InternChecklistEditor = (*service.PersonalTaskService)(nil)
	_ StaffSubmissionReader = (*service.ProgressService)(nil)
	_ Assistant             = (*service.AssistantService)(nil)
	_ Admin                 = (*service.AdminService)(nil)
	_ TaskComments          = (*service.TaskCommentService)(nil)
)

type Dependencies struct {
	// Ready проверяет готовность: БД доступна, миграции и каталог чек-листов загружены. nil — проверка не задана.
	Ready         func(context.Context) error
	Feedback      FeedbackManager
	Auth          AuthService
	Users         UserRepository
	JWT           *security.JWTService
	Team          TeamReader
	Checklists    ChecklistReader
	Items         ChecklistItemService
	Submissions   BuddySubmissionService
	PersonalTasks PersonalTaskCreator
	// ChecklistEditor and StaffSubmissions are optional: their routes are registered only when set.
	ChecklistEditor  InternChecklistEditor
	StaffSubmissions StaffSubmissionReader
	Hints            HintReader
	Personal         *service.PersonalService
	Corporate        *service.CorporateService
	SessionCheck     SessionChecker
	Sessions         SessionService
	Assistant        Assistant
	Admin            Admin
	TaskComments     TaskComments
}
