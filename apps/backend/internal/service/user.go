package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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

func (s *UserService) AssignTeamlead(ctx context.Context, buddyID, internID, teamleadID uuid.UUID) error {
	if buddyID == uuid.Nil || teamleadID == uuid.Nil {
		return ErrInvalidProfile
	}
	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		users := repository.NewUserRepository(tx)
		lead, err := users.GetByID(ctx, teamleadID)
		if err != nil {
			return err
		}
		if lead.Role != model.RoleTeamlead {
			return ErrUserNotTeamlead
		}
		if err := users.AssignTeamlead(ctx, internID, buddyID, teamleadID); err != nil {
			if err == repository.ErrNotFound {
				return ErrInternNotAssigned
			}
			return err
		}
		return nil
	})
}

func (s *UserService) CreateIntern(
	ctx context.Context,
	email string,
	passwordHash string,
	profession model.Profession,
	buddyID uuid.UUID,
) (*model.User, error) {
	return s.createIntern(ctx, email, passwordHash, profession, buddyID, uuid.Nil, "", "", "", "", "")
}

func (s *UserService) CreateInternWithProfile(ctx context.Context, email, passwordHash string, profession model.Profession, buddyID uuid.UUID, firstName, lastName, telegram, department, team string) (*model.User, error) {
	if strings.TrimSpace(firstName) == "" || strings.TrimSpace(lastName) == "" || strings.TrimSpace(department) == "" || strings.TrimSpace(team) == "" {
		return nil, ErrInvalidProfile
	}
	return s.createIntern(ctx, email, passwordHash, profession, buddyID, uuid.Nil, firstName, lastName, telegram, department, team)
}

func (s *UserService) CreateInternWithAssignments(ctx context.Context, email, passwordHash string, profession model.Profession, buddyID, teamleadID uuid.UUID, birthDate, employedSince time.Time, firstName, lastName, telegram, department, team string) (*model.User, error) {
	if strings.TrimSpace(firstName) == "" || strings.TrimSpace(lastName) == "" || strings.TrimSpace(department) == "" || strings.TrimSpace(team) == "" {
		return nil, ErrInvalidProfile
	}
	if teamleadID == uuid.Nil || birthDate.IsZero() || employedSince.IsZero() || birthDate.After(time.Now().AddDate(-16, 0, 0)) || employedSince.After(time.Now()) || employedSince.Before(birthDate) {
		return nil, ErrInvalidProfile
	}
	return s.createInternWithDates(ctx, email, passwordHash, profession, buddyID, teamleadID, birthDate, employedSince, firstName, lastName, telegram, department, team)
}

func (s *UserService) createIntern(ctx context.Context, email, passwordHash string, profession model.Profession, buddyID, teamleadID uuid.UUID, firstName, lastName, telegram, department, team string) (*model.User, error) {
	return s.createInternWithDates(ctx, email, passwordHash, profession, buddyID, teamleadID, time.Time{}, time.Time{}, firstName, lastName, telegram, department, team)
}
func (s *UserService) createInternWithDates(ctx context.Context, email, passwordHash string, profession model.Profession, buddyID, teamleadID uuid.UUID, birthDate, employedSince time.Time, firstName, lastName, telegram, department, team string) (*model.User, error) {
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
		if teamleadID != uuid.Nil {
			lead, err := userRepo.GetByID(ctx, teamleadID)
			if err != nil {
				return fmt.Errorf("get teamlead: %w", err)
			}
			if lead.Role != model.RoleTeamlead {
				return ErrUserNotTeamlead
			}
		}

		user := &model.User{
			Email:        email,
			PasswordHash: passwordHash,
			Role:         model.RoleIntern,
			Profession:   &profession,
			BuddyID:      &buddyID,
		}
		if teamleadID != uuid.Nil {
			user.TeamleadID = &teamleadID
		}
		if !birthDate.IsZero() {
			user.BirthDate = &birthDate
		}
		if !employedSince.IsZero() {
			user.EmployedSince = &employedSince
		}
		if firstName != "" {
			user.FirstName = &firstName
			user.LastName = &lastName
			user.TelegramUsername = &telegram
			user.Department = &department
			user.Team = &team
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
					Position:        i + 1,
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
