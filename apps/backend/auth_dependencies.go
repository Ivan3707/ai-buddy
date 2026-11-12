package main

import (
	"context"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type authService interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
}

type handlerUserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

type handlerDependencies struct {
	auth          authService
	users         handlerUserRepository
	jwt           *security.JWTService
	interns       internCreator
	checklists    handler.ChecklistReader
	items         handler.ChecklistItemService
	submissions   handler.BuddySubmissionService
	personalTasks handler.PersonalTaskCreator
	hints         handler.HintReader
	personal      *service.PersonalService
	corporate     *service.CorporateService
	sessionCheck  interface {
		IsActive(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	}
	sessions interface {
		Refresh(context.Context, string) (string, string, int64, error)
		Logout(context.Context, uuid.UUID, string) error
	}
}

type internCreator interface {
	CreateIntern(
		ctx context.Context,
		email string,
		passwordHash string,
		profession model.Profession,
		buddyID uuid.UUID,
	) (*model.User, error)
}
