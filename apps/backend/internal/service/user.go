package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

type UserService struct {
	db *sql.DB
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateIntern(
	ctx context.Context,
	email string,
	passwordHash string,
	profession model.Profession,
	buddyID uuid.UUID,
) (*model.User, error) {
	switch profession {
	case model.ProfessionBackend,
		model.ProfessionFrontend,
		model.ProfessionML:
	default:
		return nil, ErrInvalidProfession
	}

	if buddyID == uuid.Nil {
		return nil, ErrUserNotBuddy
	}

	var createdUser *model.User

	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		userRepo := repository.NewUserRepository(tx)
		checklistRepo := repository.NewChecklistRepository(tx)
		progressRepo := repository.NewProgressRepository(tx)

		buddy, err := userRepo.GetByID(ctx, buddyID)
		if err != nil {
			return fmt.Errorf("get buddy: %w", err)
		}

		if buddy.Role != model.RoleBuddy {
			return ErrUserNotBuddy
		}

		user := &model.User{
			Email:        email,
			PasswordHash: passwordHash,
			Role:         model.RoleIntern,
			Profession:   &profession,
			BuddyID:      &buddyID,
		}

		if err := userRepo.Create(ctx, user); err != nil {
			return err
		}

		commonChecklist, err := checklistRepo.GetCommon(ctx)
		if err != nil {
			return fmt.Errorf("get common checklist: %w", err)
		}

		professionalChecklist, err := checklistRepo.GetProfessional(ctx, profession)
		if err != nil {
			return fmt.Errorf("get professional checklist: %w", err)
		}

		commonItems, err := checklistRepo.GetItems(ctx, commonChecklist.ID)
		if err != nil {
			return fmt.Errorf("get common checklist items: %w", err)
		}

		professionalItems, err := checklistRepo.GetItems(ctx, professionalChecklist.ID)
		if err != nil {
			return fmt.Errorf("get professional checklist items: %w", err)
		}

		progress := make([]model.UserChecklistProgress, 0,
			len(commonItems)+len(professionalItems),
		)

		appendChecklistProgress := func(
			checklist *model.Checklist,
			items []model.ChecklistItem,
		) {
			for i, item := range items {
				status := model.StatusUnavailable
				if i == 0 {
					status = model.StatusActive
				}

				progress = append(progress, model.UserChecklistProgress{
					UserID:          user.ID,
					ChecklistID:     checklist.ID,
					ChecklistItemID: item.ID,
					Status:          status,
				})
			}
		}

		appendChecklistProgress(commonChecklist, commonItems)
		appendChecklistProgress(professionalChecklist, professionalItems)

		if len(progress) > 0 {
			if err := progressRepo.CreateMany(ctx, progress); err != nil {
				return fmt.Errorf("create intern progress: %w", err)
			}
		}

		createdUser = user
		return nil
	})
	if err != nil {
		return nil, err
	}

	return createdUser, nil
}
