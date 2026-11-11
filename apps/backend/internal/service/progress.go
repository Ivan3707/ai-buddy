package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

type ProgressService struct {
	db *sql.DB
}

type PendingSubmission struct {
	Submission model.ChecklistItemSubmission
	InternID   uuid.UUID
	ItemID     uuid.UUID
	ItemTitle  string
}

func NewProgressService(db *sql.DB) *ProgressService {
	return &ProgressService{db: db}
}

func (s *ProgressService) CompleteItem(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
) error {
	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		progressRepo := repository.NewProgressRepository(tx)
		checklistRepo := repository.NewChecklistRepository(tx)

		currentProgress, err := progressRepo.GetByUserAndItem(
			ctx,
			userID,
			itemID,
		)
		if err != nil {
			return fmt.Errorf("get current item progress: %w", err)
		}

		items, currentIndex, err := getChecklistItemContext(
			ctx,
			checklistRepo,
			currentProgress.ChecklistID,
			itemID,
		)
		if err != nil {
			return err
		}

		if items[currentIndex].RequiresReview {
			return ErrSubmissionRequired
		}

		switch currentProgress.Status {
		case model.StatusCompleted, model.StatusVerified:
			return nil
		case model.StatusActive:
			// Continue with the completion transaction.
		default:
			return ErrItemNotActive
		}

		now := time.Now().UTC()
		if err := progressRepo.UpdateStatus(
			ctx,
			userID,
			itemID,
			model.StatusCompleted,
			&now,
		); err != nil {
			return fmt.Errorf("complete checklist item: %w", err)
		}

		if err := openNextChecklistItem(
			ctx,
			progressRepo,
			userID,
			currentProgress.ChecklistID,
			items,
			currentIndex,
		); err != nil {
			return err
		}

		return nil
	})
}

func (s *ProgressService) SubmitItem(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
	resultText *string,
	resultURL *string,
) (model.ChecklistItemSubmission, error) {
	if !hasNonEmptyValue(resultText) && !hasNonEmptyValue(resultURL) {
		return model.ChecklistItemSubmission{}, ErrInvalidSubmission
	}

	var created model.ChecklistItemSubmission

	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		userRepo := repository.NewUserRepository(tx)
		progressRepo := repository.NewProgressRepository(tx)
		checklistRepo := repository.NewChecklistRepository(tx)
		submissionRepo := repository.NewSubmissionRepository(tx)

		user, err := userRepo.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("get submission user: %w", err)
		}
		if user.Role != model.RoleIntern {
			return ErrUserNotIntern
		}

		currentProgress, err := progressRepo.GetByUserAndItem(
			ctx,
			userID,
			itemID,
		)
		if err != nil {
			return fmt.Errorf("get submission progress: %w", err)
		}
		if currentProgress.Status != model.StatusActive {
			return ErrItemNotActive
		}

		items, currentIndex, err := getChecklistItemContext(
			ctx,
			checklistRepo,
			currentProgress.ChecklistID,
			itemID,
		)
		if err != nil {
			return err
		}
		if !items[currentIndex].RequiresReview {
			return ErrInvalidSubmission
		}

		_, err = submissionRepo.GetPendingByProgressID(
			ctx,
			currentProgress.ID,
		)
		switch {
		case err == nil:
			return fmt.Errorf(
				"%w: pending submission already exists",
				ErrInvalidSubmission,
			)
		case !errors.Is(err, repository.ErrNotFound):
			return fmt.Errorf("check pending submission: %w", err)
		}

		attemptNumber, err := submissionRepo.GetNextAttemptNumber(
			ctx,
			currentProgress.ID,
		)
		if err != nil {
			return fmt.Errorf("get next submission attempt: %w", err)
		}

		now := time.Now().UTC()
		submission, err := submissionRepo.Create(
			ctx,
			currentProgress.ID,
			attemptNumber,
			resultText,
			resultURL,
			now,
		)
		if err != nil {
			return fmt.Errorf("create item submission: %w", err)
		}

		if err := progressRepo.UpdateStatus(
			ctx,
			userID,
			itemID,
			model.StatusCompleted,
			&now,
		); err != nil {
			return fmt.Errorf("complete submitted checklist item: %w", err)
		}

		if err := openNextChecklistItem(
			ctx,
			progressRepo,
			userID,
			currentProgress.ChecklistID,
			items,
			currentIndex,
		); err != nil {
			return err
		}

		created = *submission
		return nil
	})
	if err != nil {
		return model.ChecklistItemSubmission{}, err
	}

	return created, nil
}

func (s *ProgressService) ReviewSubmission(
	ctx context.Context,
	buddyID uuid.UUID,
	submissionID uuid.UUID,
	approved bool,
	comment *string,
) error {
	if !approved && !hasNonEmptyValue(comment) {
		return ErrInvalidSubmission
	}

	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		userRepo := repository.NewUserRepository(tx)
		progressRepo := repository.NewProgressRepository(tx)
		checklistRepo := repository.NewChecklistRepository(tx)
		submissionRepo := repository.NewSubmissionRepository(tx)

		unlockedSubmission, err := submissionRepo.GetByID(ctx, submissionID)
		if err != nil {
			return fmt.Errorf("get submission: %w", err)
		}

		currentProgress, err := progressRepo.GetByIDForUpdate(
			ctx,
			unlockedSubmission.ProgressID,
		)
		if err != nil {
			return fmt.Errorf("lock submission progress: %w", err)
		}

		submission, err := submissionRepo.GetByIDForUpdate(ctx, submissionID)
		if err != nil {
			return fmt.Errorf("lock submission: %w", err)
		}
		if submission.ProgressID != currentProgress.ID {
			return fmt.Errorf(
				"%w: submission progress changed during review",
				ErrProgressIncomplete,
			)
		}
		if submission.Status != model.SubmissionPending {
			return ErrSubmissionAlreadyReviewed
		}
		if currentProgress.Status != model.StatusCompleted {
			return ErrReviewNotPending
		}
		if currentProgress.CompletedAt == nil {
			return fmt.Errorf(
				"%w: completed item has no completion timestamp",
				ErrProgressIncomplete,
			)
		}

		items, currentIndex, err := getChecklistItemContext(
			ctx,
			checklistRepo,
			currentProgress.ChecklistID,
			currentProgress.ChecklistItemID,
		)
		if err != nil {
			return err
		}
		if !items[currentIndex].RequiresReview {
			return ErrReviewNotPending
		}

		if buddyID == currentProgress.UserID {
			return ErrCannotReviewSelf
		}

		buddy, err := userRepo.GetByID(ctx, buddyID)
		if err != nil {
			return fmt.Errorf("get buddy: %w", err)
		}
		if buddy.Role != model.RoleBuddy {
			return ErrUserNotBuddy
		}

		intern, err := userRepo.GetByID(ctx, currentProgress.UserID)
		if err != nil {
			return fmt.Errorf("get intern: %w", err)
		}
		if intern.Role != model.RoleIntern {
			return ErrUserNotIntern
		}
		if intern.BuddyID == nil || *intern.BuddyID != buddyID {
			return ErrInternNotAssigned
		}

		status := model.SubmissionRejected
		var completedAt *time.Time
		if approved {
			status = model.SubmissionApproved
			completedAt = currentProgress.CompletedAt
		}

		reviewedAt := time.Now().UTC()
		reviewed, err := submissionRepo.Review(
			ctx,
			submissionID,
			status,
			reviewedAt,
			buddyID,
			comment,
		)
		if err != nil {
			return fmt.Errorf("review submission: %w", err)
		}
		if !reviewed {
			return ErrSubmissionAlreadyReviewed
		}

		progressStatus := model.StatusActive
		if approved {
			progressStatus = model.StatusVerified
		}
		if err := progressRepo.UpdateStatus(
			ctx,
			currentProgress.UserID,
			currentProgress.ChecklistItemID,
			progressStatus,
			completedAt,
		); err != nil {
			return fmt.Errorf("update reviewed item progress: %w", err)
		}

		return nil
	})
}

func (s *ProgressService) ListSubmissions(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
) ([]model.ChecklistItemSubmission, error) {
	userRepo := repository.NewUserRepository(s.db)
	user, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get submission history user: %w", err)
	}
	if user.Role != model.RoleIntern {
		return nil, ErrUserNotIntern
	}

	progress, err := repository.NewProgressRepository(s.db).
		GetByUserAndItemRead(ctx, userID, itemID)
	if err != nil {
		return nil, fmt.Errorf("get submission history progress: %w", err)
	}

	submissions, err := repository.NewSubmissionRepository(s.db).
		ListByProgressID(ctx, progress.ID)
	if err != nil {
		return nil, fmt.Errorf("list submission history: %w", err)
	}

	return submissions, nil
}

func (s *ProgressService) ListPendingSubmissions(
	ctx context.Context,
	buddyID uuid.UUID,
) ([]PendingSubmission, error) {
	user, err := repository.NewUserRepository(s.db).GetByID(ctx, buddyID)
	if err != nil {
		return nil, fmt.Errorf("get pending submissions buddy: %w", err)
	}
	if user.Role != model.RoleBuddy {
		return nil, ErrUserNotBuddy
	}

	rows, err := repository.NewSubmissionRepository(s.db).
		ListPendingByBuddyID(ctx, buddyID)
	if err != nil {
		return nil, fmt.Errorf("list buddy pending submissions: %w", err)
	}

	result := make([]PendingSubmission, 0, len(rows))
	for _, row := range rows {
		result = append(result, PendingSubmission{
			Submission: row.Submission,
			InternID:   row.InternID,
			ItemID:     row.ItemID,
			ItemTitle:  row.ItemTitle,
		})
	}

	return result, nil
}

// ReviewItem is retained for non-HTTP compatibility. HTTP review addresses a
// concrete submission through ReviewSubmission.
func (s *ProgressService) ReviewItem(
	ctx context.Context,
	buddyID uuid.UUID,
	internID uuid.UUID,
	itemID uuid.UUID,
	approved bool,
) error {
	progressRepo := repository.NewProgressRepository(s.db)
	submissionRepo := repository.NewSubmissionRepository(s.db)

	progress, err := progressRepo.GetByUserAndItem(ctx, internID, itemID)
	if err != nil {
		return fmt.Errorf("get review item progress: %w", err)
	}

	submission, err := submissionRepo.GetPendingByProgressID(ctx, progress.ID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrReviewNotPending
	}
	if err != nil {
		return fmt.Errorf("get pending item submission: %w", err)
	}

	return s.ReviewSubmission(ctx, buddyID, submission.ID, approved, nil)
}

func getChecklistItemContext(
	ctx context.Context,
	checklistRepo *repository.ChecklistRepository,
	checklistID uuid.UUID,
	itemID uuid.UUID,
) ([]model.ChecklistItem, int, error) {
	items, err := checklistRepo.GetItems(ctx, checklistID)
	if err != nil {
		return nil, -1, fmt.Errorf("get checklist items: %w", err)
	}

	for i := range items {
		if items[i].ID == itemID {
			return items, i, nil
		}
	}

	return nil, -1, fmt.Errorf(
		"%w: item %s not found in checklist",
		ErrProgressIncomplete,
		itemID,
	)
}

func openNextChecklistItem(
	ctx context.Context,
	progressRepo *repository.ProgressRepository,
	userID uuid.UUID,
	checklistID uuid.UUID,
	items []model.ChecklistItem,
	currentIndex int,
) error {
	if currentIndex+1 >= len(items) {
		return nil
	}

	nextItem := items[currentIndex+1]
	nextProgress, err := progressRepo.GetByUserAndItem(
		ctx,
		userID,
		nextItem.ID,
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf(
				"%w: missing progress for next item %s",
				ErrProgressIncomplete,
				nextItem.ID,
			)
		}
		return fmt.Errorf("get next item progress: %w", err)
	}

	if nextProgress.ChecklistID != checklistID {
		return fmt.Errorf(
			"%w: next item %s belongs to another checklist",
			ErrProgressIncomplete,
			nextItem.ID,
		)
	}

	switch nextProgress.Status {
	case model.StatusUnavailable:
		if err := progressRepo.UpdateStatus(
			ctx,
			userID,
			nextItem.ID,
			model.StatusActive,
			nil,
		); err != nil {
			return fmt.Errorf("activate next checklist item: %w", err)
		}
	case model.StatusActive, model.StatusCompleted, model.StatusVerified:
		// A later item can already be open after an earlier attempt was rejected.
	default:
		return fmt.Errorf(
			"%w: next item %s has unexpected status %q",
			ErrProgressIncomplete,
			nextItem.ID,
			nextProgress.Status,
		)
	}

	return nil
}

func hasNonEmptyValue(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}
