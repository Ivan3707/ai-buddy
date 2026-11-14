package handler

import (
	"context"
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

type PersonalTaskCreator interface {
	Create(context.Context, uuid.UUID, uuid.UUID, int, string, string, string, int, bool, ...*time.Time) (uuid.UUID, error)
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

type Dependencies struct {
	Feedback      FeedbackManager
	Auth          AuthService
	Users         UserRepository
	JWT           *security.JWTService
	Team          TeamReader
	Checklists    ChecklistReader
	Items         ChecklistItemService
	Submissions   BuddySubmissionService
	PersonalTasks PersonalTaskCreator
	Hints         HintReader
	Personal      *service.PersonalService
	Corporate     *service.CorporateService
	SessionCheck  SessionChecker
	Sessions      SessionService
	Assistant     *service.AssistantService
	Admin         *service.AdminService
}
