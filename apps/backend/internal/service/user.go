package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// UserService only reads team data. Accounts are created exclusively by
// HRSynchronizer from the Mock HR snapshot (ARCH-04 / A-01); onboarding
// progress is initialized by InitializeInternProgress.
type UserService struct {
	db *sql.DB
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) ListAssignedInterns(ctx context.Context, actorID uuid.UUID, role model.UserRole) ([]model.User, error) {
	if role != model.RoleBuddy && role != model.RoleTeamlead {
		return nil, ErrUserNotStaff
	}
	return repository.NewUserRepository(s.db).ListAssignedInterns(ctx, actorID, role)
}

func (s *UserService) ListTeamleads(ctx context.Context) ([]model.User, error) {
	return repository.NewUserRepository(s.db).ListByRole(ctx, model.RoleTeamlead)
}

func (s *UserService) ListBuddies(ctx context.Context) ([]model.User, error) {
	return repository.NewUserRepository(s.db).ListByRole(ctx, model.RoleBuddy)
}
