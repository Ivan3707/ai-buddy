package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

var (
	ErrInvalidPosition = errors.New("invalid task position")
	// ErrInvalidTask: an edit with nothing to change, an empty title or a non-positive estimate.
	ErrInvalidTask = errors.New("invalid task")
	// ErrInvalidChecklistType: checklist_type is neither common nor professional.
	ErrInvalidChecklistType = errors.New("invalid checklist type")
)

// PersonalTaskService is the teamlead's editor of an intern's checklists (docs/intern-checklist-edits.md):
// add a personal task, change or remove any task of that one intern. The shared catalog is never changed.
// Every method checks that the intern is assigned to this teamlead (users.teamlead_id).
type PersonalTaskService struct{ db *sql.DB }

func NewPersonalTaskService(db *sql.DB) *PersonalTaskService { return &PersonalTaskService{db} }

// TaskEdit is a partial update of one task of one intern; nil fields keep their current value.
type TaskEdit struct {
	Title            *string
	Description      *string
	ExpectedResult   *string
	EstimatedMinutes *int
	DueAt            *time.Time
}

// Create inserts a personal task into the intern's professional checklist (kept for existing callers).
func (s *PersonalTaskService) Create(ctx context.Context, teamleadID, internID uuid.UUID, position int, title, description, expected string, minutes int, review bool, dueAt ...*time.Time) (uuid.UUID, error) {
	var deadline *time.Time
	if len(dueAt) > 0 {
		deadline = dueAt[0]
	}
	return s.CreateInChecklist(ctx, teamleadID, internID, model.ChecklistTypeProfessional, position, title, description, expected, minutes, review, deadline)
}

// CreateInChecklist inserts a personal task at a 1-based position of the intern's common or professional checklist.
func (s *PersonalTaskService) CreateInChecklist(ctx context.Context, teamleadID, internID uuid.UUID, checklistType model.ChecklistType, position int, title, description, expected string, minutes int, review bool, dueAt *time.Time) (uuid.UUID, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(expected) == "" || minutes <= 0 || position < 1 {
		return uuid.Nil, ErrInvalidPosition
	}
	if checklistType != model.ChecklistTypeCommon && checklistType != model.ChecklistTypeProfessional {
		return uuid.Nil, ErrInvalidChecklistType
	}
	var id uuid.UUID
	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		repo := repository.NewPersonalTaskRepository(tx)
		if err := repo.LockUser(ctx, internID); err != nil {
			return err
		}
		user, err := teamleadIntern(ctx, tx, teamleadID, internID)
		if err != nil {
			return err
		}
		checklists := repository.NewChecklistRepository(tx)
		var checklist *model.Checklist
		if checklistType == model.ChecklistTypeCommon {
			checklist, err = checklists.GetCommon(ctx)
		} else {
			checklist, err = checklists.GetProfessional(ctx, *user.Profession)
		}
		if err != nil {
			return err
		}
		id, err = repo.Insert(ctx, internID, checklist.ID, position, strings.TrimSpace(title), description, strings.TrimSpace(expected), minutes, review, dueAt)
		if errors.Is(err, repository.ErrInvalidPosition) {
			return ErrInvalidPosition
		}
		return err
	})
	return id, err
}

// UpdateTask changes the wording, estimate or deadline of one task for this intern only. A catalog task
// gets per-intern overrides; the catalog row and other interns are not affected.
func (s *PersonalTaskService) UpdateTask(ctx context.Context, teamleadID, internID, itemID uuid.UUID, edit TaskEdit) error {
	if edit.Title == nil && edit.Description == nil && edit.ExpectedResult == nil && edit.EstimatedMinutes == nil && edit.DueAt == nil {
		return ErrInvalidTask
	}
	if edit.Title != nil {
		title := strings.TrimSpace(*edit.Title)
		if title == "" || len([]rune(title)) > 255 {
			return ErrInvalidTask
		}
		edit.Title = &title
	}
	if edit.EstimatedMinutes != nil && *edit.EstimatedMinutes <= 0 {
		return ErrInvalidTask
	}
	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		repo := repository.NewInternChecklistRepository(tx)
		progress, err := s.lockInternTask(ctx, tx, repo, teamleadID, internID, itemID)
		if errors.Is(err, errTaskHidden) {
			return repository.ErrNotFound
		}
		if err != nil {
			return err
		}
		return repo.SetOverrides(ctx, progress.ID, repository.TaskOverrides{
			Title: edit.Title, Description: edit.Description, ExpectedResult: edit.ExpectedResult,
			EstimatedMinutes: edit.EstimatedMinutes, DueAt: edit.DueAt,
		})
	})
}

// RemoveTask removes one task from this intern's checklist (soft delete, idempotent). When it was the
// intern's current shared task, the next one becomes active so the intern is never stuck.
func (s *PersonalTaskService) RemoveTask(ctx context.Context, teamleadID, internID, itemID uuid.UUID) error {
	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		repo := repository.NewInternChecklistRepository(tx)
		progress, err := s.lockInternTask(ctx, tx, repo, teamleadID, internID, itemID)
		if errors.Is(err, errTaskHidden) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := repo.Hide(ctx, progress.ID); err != nil {
			return err
		}
		if progress.Personal {
			return nil
		}
		return repo.ActivateFirstOpen(ctx, internID, progress.ChecklistID)
	})
}

var errTaskHidden = errors.New("task already removed")

// lockInternTask checks the teamlead's right to the intern and locks the intern's progress row of the item.
// A removed task is reported as errTaskHidden (callers turn it into 404 or a no-op).
func (s *PersonalTaskService) lockInternTask(ctx context.Context, tx *sql.Tx, repo *repository.InternChecklistRepository, teamleadID, internID, itemID uuid.UUID) (*repository.EditableProgress, error) {
	if err := repository.NewPersonalTaskRepository(tx).LockUser(ctx, internID); err != nil {
		return nil, err
	}
	if _, err := teamleadIntern(ctx, tx, teamleadID, internID); err != nil {
		return nil, err
	}
	progress, err := repo.LockProgress(ctx, internID, itemID)
	if err != nil {
		return nil, err
	}
	if err := ensureCurrentTrack(ctx, tx, internID, progress.ChecklistID); err != nil {
		return nil, err
	}
	if progress.Hidden {
		return progress, errTaskHidden
	}
	return progress, nil
}

// teamleadIntern loads the intern and checks that teamleadID is their teamlead.
func teamleadIntern(ctx context.Context, tx *sql.Tx, teamleadID, internID uuid.UUID) (*model.User, error) {
	user, err := repository.NewUserRepository(tx).GetByID(ctx, internID)
	if err != nil {
		return nil, err
	}
	if user.Role != model.RoleIntern || user.TeamleadID == nil || *user.TeamleadID != teamleadID {
		return nil, ErrInternNotAssigned
	}
	if user.Profession == nil {
		return nil, ErrProfessionNotSet
	}
	return user, nil
}
