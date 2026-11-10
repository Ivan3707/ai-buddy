package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

type ProgressService struct {
	db *sql.DB
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

		switch currentProgress.Status {
		case model.StatusCompleted, model.StatusVerified:
			return nil
		case model.StatusActive:
			// Продолжаем выполнение.
		default:
			return ErrItemNotActive
		}

		items, err := checklistRepo.GetItems(
			ctx,
			currentProgress.ChecklistID,
		)
		if err != nil {
			return fmt.Errorf("get checklist items: %w", err)
		}

		currentIndex := -1

		for i, item := range items {
			if item.ID == itemID {
				currentIndex = i
				break
			}
		}

		if currentIndex == -1 {
			return fmt.Errorf(
				"%w: current item %s not found in checklist",
				ErrProgressIncomplete,
				itemID,
			)
		}

		currentItem := items[currentIndex]

		var nextItem *model.ChecklistItem

		if currentIndex+1 < len(items) {
			next := items[currentIndex+1]
			nextItem = &next

			nextProgress, err := progressRepo.GetByUserAndItem(
				ctx,
				userID,
				next.ID,
			)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return fmt.Errorf(
						"%w: missing progress for next item %s",
						ErrProgressIncomplete,
						next.ID,
					)
				}

				return fmt.Errorf("get next item progress: %w", err)
			}

			if nextProgress.ChecklistID != currentProgress.ChecklistID ||
				nextProgress.Status != model.StatusUnavailable {
				return fmt.Errorf(
					"%w: next item %s has unexpected progress state",
					ErrProgressIncomplete,
					next.ID,
				)
			}
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

		// Задание с проверкой ждёт решения buddy.
		// Следующее задание пока остаётся unavailable.
		if currentItem.RequiresReview {
			return nil
		}

		if nextItem != nil {
			if err := progressRepo.UpdateStatus(
				ctx,
				userID,
				nextItem.ID,
				model.StatusActive,
				nil,
			); err != nil {
				return fmt.Errorf("activate next checklist item: %w", err)
			}
		}

		return nil
	})
}

func (s *ProgressService) ReviewItem(
	ctx context.Context,
	buddyID uuid.UUID,
	internID uuid.UUID,
	itemID uuid.UUID,
	approved bool,
) error {
	return withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		userRepo := repository.NewUserRepository(tx)
		progressRepo := repository.NewProgressRepository(tx)
		checklistRepo := repository.NewChecklistRepository(tx)

		if buddyID == internID {
			return ErrCannotReviewSelf
		}

		buddy, err := userRepo.GetByID(ctx, buddyID)
		if err != nil {
			return fmt.Errorf("get buddy: %w", err)
		}
		if buddy.Role != model.RoleBuddy {
			return ErrUserNotBuddy
		}

		intern, err := userRepo.GetByID(ctx, internID)
		if err != nil {
			return fmt.Errorf("get intern: %w", err)
		}
		if intern.Role != model.RoleIntern {
			return ErrUserNotIntern
		}

		if intern.BuddyID == nil || *intern.BuddyID != buddyID {
			return ErrInternNotAssigned
		}

		currentProgress, err := progressRepo.GetByUserAndItem(
			ctx,
			internID,
			itemID,
		)
		if err != nil {
			return fmt.Errorf("get item progress: %w", err)
		}

		if currentProgress.Status != model.StatusCompleted {
			return ErrReviewNotPending
		}

		items, err := checklistRepo.GetItems(ctx, currentProgress.ChecklistID)
		if err != nil {
			return fmt.Errorf("get checklist items: %w", err)
		}

		currentIndex := -1
		for i, item := range items {
			if item.ID == itemID {
				currentIndex = i
				if !item.RequiresReview {
					return ErrReviewNotPending
				}
				break
			}
		}

		if currentIndex == -1 {
			return fmt.Errorf(
				"%w: item %s not found in checklist",
				ErrProgressIncomplete,
				itemID,
			)
		}

		if !approved {
			if err := progressRepo.UpdateStatus(
				ctx,
				internID,
				itemID,
				model.StatusActive,
				nil,
			); err != nil {
				return fmt.Errorf("return item for revision: %w", err)
			}

			return nil
		}

		// Следующее задание проверяем до изменения текущего,
		// чтобы при повреждённом прогрессе транзакция откатилась.
		var nextItem *model.ChecklistItem

		if currentIndex+1 < len(items) {
			next := items[currentIndex+1]
			nextItem = &next

			nextProgress, err := progressRepo.GetByUserAndItem(
				ctx,
				internID,
				next.ID,
			)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return fmt.Errorf(
						"%w: missing progress for next item %s",
						ErrProgressIncomplete,
						next.ID,
					)
				}
				return fmt.Errorf("get next item progress: %w", err)
			}

			if nextProgress.ChecklistID != currentProgress.ChecklistID ||
				nextProgress.Status != model.StatusUnavailable {
				return fmt.Errorf(
					"%w: next item %s has unexpected progress state",
					ErrProgressIncomplete,
					next.ID,
				)
			}
		}

		if currentProgress.CompletedAt == nil {
			return fmt.Errorf(
				"%w: completed item has no completion timestamp",
				ErrProgressIncomplete,
			)
		}

		if err := progressRepo.UpdateStatus(
			ctx,
			internID,
			itemID,
			model.StatusVerified,
			currentProgress.CompletedAt,
		); err != nil {
			return fmt.Errorf("verify checklist item: %w", err)
		}

		if nextItem != nil {
			if err := progressRepo.UpdateStatus(
				ctx,
				internID,
				nextItem.ID,
				model.StatusActive,
				nil,
			); err != nil {
				return fmt.Errorf("activate next checklist item: %w", err)
			}
		}

		return nil
	})
}
