package service

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"strings"
)

var ErrInvalidPosition = errors.New("invalid task position")

type PersonalTaskService struct{ db *sql.DB }

func NewPersonalTaskService(db *sql.DB) *PersonalTaskService { return &PersonalTaskService{db} }
func (s *PersonalTaskService) Create(ctx context.Context, teamleadID, internID uuid.UUID, position int, title, description, expected string, minutes int, review bool) (uuid.UUID, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(expected) == "" || minutes <= 0 || position < 1 {
		return uuid.Nil, ErrInvalidPosition
	}
	var id uuid.UUID
	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		repo := repository.NewPersonalTaskRepository(tx)
		if err := repo.LockUser(ctx, internID); err != nil {
			return err
		}
		user, err := repository.NewUserRepository(tx).GetByID(ctx, internID)
		if err != nil {
			return err
		}
		if user.Role != model.RoleIntern || user.TeamleadID == nil || *user.TeamleadID != teamleadID {
			return ErrInternNotAssigned
		}
		if user.Profession == nil {
			return ErrProfessionNotSet
		}
		checklist, err := repository.NewChecklistRepository(tx).GetProfessional(ctx, *user.Profession)
		if err != nil {
			return err
		}
		id, err = repo.Insert(ctx, internID, checklist.ID, position, strings.TrimSpace(title), description, strings.TrimSpace(expected), minutes, review)
		if errors.Is(err, repository.ErrInvalidPosition) {
			return ErrInvalidPosition
		}
		return err
	})
	return id, err
}
