package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

type ChecklistItemProgress struct {
	Item        model.ChecklistItem
	Status      model.ChecklistStatus
	CompletedAt *time.Time
	// ReturnedForRework: the buddy rejected the latest submission and the intern has not resubmitted;
	// the item is active again. ReworkComment is that rejection comment.
	ReturnedForRework bool
	ReworkComment     *string
}

type ChecklistStageWithProgress struct {
	Stage model.ChecklistStage
	Items []ChecklistItemProgress
}

type ChecklistWithProgress struct {
	Checklist         model.Checklist
	Stages            []ChecklistStageWithProgress
	Items             []ChecklistItemProgress
	TotalItems        int
	CompletedItems    int
	CompletionPercent float64
}

type UserChecklists struct {
	Common       ChecklistWithProgress
	Professional ChecklistWithProgress
}

type ChecklistService struct {
	db *sql.DB
}

func NewChecklistService(db *sql.DB) *ChecklistService {
	return &ChecklistService{db: db}
}

func (s *ChecklistService) GetByUserID(
	ctx context.Context,
	userID uuid.UUID,
) (*UserChecklists, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("begin checklist read transaction: %w", err)
	}
	defer tx.Rollback()

	userRepo := repository.NewUserRepository(tx)
	checklistRepo := repository.NewChecklistRepository(tx)
	progressRepo := repository.NewProgressRepository(tx)

	user, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if user.Role != model.RoleIntern {
		return nil, ErrUserNotIntern
	}
	if user.Profession == nil {
		return nil, ErrProfessionNotSet
	}

	common, err := checklistRepo.GetCommon(ctx)
	if err != nil {
		return nil, fmt.Errorf("get common checklist: %w", err)
	}

	professional, err := checklistRepo.GetProfessional(ctx, *user.Profession)
	if err != nil {
		return nil, fmt.Errorf("get professional checklist: %w", err)
	}

	commonStages, err := checklistRepo.GetStages(ctx, common.ID)
	if err != nil {
		return nil, fmt.Errorf("get common checklist stages: %w", err)
	}

	professionalStages, err := checklistRepo.GetStages(ctx, professional.ID)
	if err != nil {
		return nil, fmt.Errorf("get professional checklist stages: %w", err)
	}

	commonItems, err := checklistRepo.GetItemsForUser(ctx, common.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("get common checklist items: %w", err)
	}

	professionalItems, err := checklistRepo.GetItemsForUser(ctx, professional.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("get professional checklist items: %w", err)
	}

	progress, err := progressRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user checklist progress: %w", err)
	}

	returned, err := repository.NewSubmissionRepository(tx).ListReturnedForRework(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get returned checklist items: %w", err)
	}

	expectedItems := make(map[uuid.UUID]uuid.UUID,
		len(commonItems)+len(professionalItems),
	)

	for _, item := range commonItems {
		expectedItems[item.ID] = common.ID
	}
	for _, item := range professionalItems {
		expectedItems[item.ID] = professional.ID
	}

	progressByItemID := make(
		map[uuid.UUID]model.UserChecklistProgress,
		len(progress),
	)

	for _, itemProgress := range progress {
		expectedChecklistID, ok := expectedItems[itemProgress.ChecklistItemID]
		if !ok {
			// HR can change profession; retain the previous track as history.
			continue
		}
		if itemProgress.ChecklistID != expectedChecklistID {
			return nil, fmt.Errorf(
				"%w: unexpected progress record for item %s",
				ErrProgressIncomplete,
				itemProgress.ChecklistItemID,
			)
		}

		progressByItemID[itemProgress.ChecklistItemID] = itemProgress
	}

	commonResult, err := buildChecklistWithProgress(
		*common,
		commonStages,
		commonItems,
		progressByItemID,
		returned,
	)
	if err != nil {
		return nil, err
	}

	professionalResult, err := buildChecklistWithProgress(
		*professional,
		professionalStages,
		professionalItems,
		progressByItemID,
		returned,
	)
	if err != nil {
		return nil, err
	}

	if len(progressByItemID) != len(expectedItems) {
		return nil, fmt.Errorf(
			"%w: expected %d progress records, got %d",
			ErrProgressIncomplete,
			len(expectedItems),
			len(progressByItemID),
		)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit checklist read transaction: %w", err)
	}

	return &UserChecklists{
		Common:       commonResult,
		Professional: professionalResult,
	}, nil
}

func buildChecklistWithProgress(
	checklist model.Checklist,
	stages []model.ChecklistStage,
	items []model.ChecklistItem,
	progressByItemID map[uuid.UUID]model.UserChecklistProgress,
	returned map[uuid.UUID]string,
) (ChecklistWithProgress, error) {
	result := ChecklistWithProgress{
		Checklist:  checklist,
		Stages:     make([]ChecklistStageWithProgress, 0, len(stages)),
		Items:      make([]ChecklistItemProgress, 0, len(items)),
		TotalItems: len(items),
	}

	stageIndexes := make(map[uuid.UUID]int, len(stages))

	for _, stage := range stages {
		stageIndexes[stage.ID] = len(result.Stages)
		result.Stages = append(result.Stages, ChecklistStageWithProgress{
			Stage: stage,
			Items: make([]ChecklistItemProgress, 0),
		})
	}

	for _, item := range items {
		itemProgress, ok := progressByItemID[item.ID]
		if !ok {
			return ChecklistWithProgress{}, fmt.Errorf(
				"%w: missing progress for checklist item %s",
				ErrProgressIncomplete,
				item.ID,
			)
		}

		stageIndex, ok := stageIndexes[item.StageID]
		if !ok {
			return ChecklistWithProgress{}, fmt.Errorf(
				"%w: stage %s not found for checklist item %s",
				ErrProgressIncomplete,
				item.StageID,
				item.ID,
			)
		}

		itemResult := ChecklistItemProgress{
			Item:        item,
			Status:      itemProgress.Status,
			CompletedAt: itemProgress.CompletedAt,
		}
		if comment, ok := returned[item.ID]; ok && itemProgress.Status == model.StatusActive {
			itemResult.ReturnedForRework = true
			if comment != "" {
				itemResult.ReworkComment = &comment
			}
		}

		if itemProgress.Status == model.StatusVerified ||
			(!item.RequiresReview && itemProgress.Status == model.StatusCompleted) {
			result.CompletedItems++
		}

		result.Items = append(result.Items, itemResult)
		result.Stages[stageIndex].Items = append(
			result.Stages[stageIndex].Items,
			itemResult,
		)
	}

	if result.TotalItems > 0 {
		result.CompletionPercent = float64(result.CompletedItems) /
			float64(result.TotalItems) * 100
	}

	return result, nil
}
