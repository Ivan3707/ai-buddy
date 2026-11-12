package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type ChecklistRepository struct {
	db DBTX
}

func NewChecklistRepository(db DBTX) *ChecklistRepository {
	return &ChecklistRepository{db: db}
}

func (r *ChecklistRepository) GetCommon(
	ctx context.Context,
) (*model.Checklist, error) {
	query := `
		SELECT id, name, type, profession, created_at
		FROM checklists
		WHERE type = $1
	`

	return r.getChecklist(ctx, query, string(model.ChecklistTypeCommon))
}

func (r *ChecklistRepository) GetProfessional(
	ctx context.Context,
	profession model.Profession,
) (*model.Checklist, error) {
	query := `
		SELECT id, name, type, profession, created_at
		FROM checklists
		WHERE type = $1 AND profession = $2
	`

	return r.getChecklist(
		ctx,
		query,
		string(model.ChecklistTypeProfessional),
		string(profession),
	)
}

func (r *ChecklistRepository) getChecklist(
	ctx context.Context,
	query string,
	args ...any,
) (*model.Checklist, error) {
	checklist := &model.Checklist{}
	var profession sql.NullString

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&checklist.ID,
		&checklist.Name,
		&checklist.Type,
		&profession,
		&checklist.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get checklist: %w", err)
	}

	if profession.Valid {
		value := model.Profession(profession.String)
		checklist.Profession = &value
	}

	return checklist, nil
}

func (r *ChecklistRepository) GetStages(
	ctx context.Context,
	checklistID uuid.UUID,
) ([]model.ChecklistStage, error) {
	query := `
		SELECT
			id,
			checklist_id,
			title,
			week_start,
			week_end,
			goal,
			expected_result,
			position,
			created_at
		FROM checklist_stages
		WHERE checklist_id = $1
		ORDER BY position ASC
	`

	rows, err := r.db.QueryContext(ctx, query, checklistID)
	if err != nil {
		return nil, fmt.Errorf("get checklist stages: %w", err)
	}
	defer rows.Close()

	stages := make([]model.ChecklistStage, 0)

	for rows.Next() {
		var stage model.ChecklistStage

		if err := rows.Scan(
			&stage.ID,
			&stage.ChecklistID,
			&stage.Title,
			&stage.WeekStart,
			&stage.WeekEnd,
			&stage.Goal,
			&stage.ExpectedResult,
			&stage.Position,
			&stage.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan checklist stage: %w", err)
		}

		stages = append(stages, stage)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checklist stages: %w", err)
	}

	return stages, nil
}

func (r *ChecklistRepository) GetItems(
	ctx context.Context,
	checklistID uuid.UUID,
) ([]model.ChecklistItem, error) {
	query := `
		SELECT
			id,
			checklist_id,
			stage_id,
			title,
			description,
			expected_result,
			requires_review,
			estimated_minutes,
			position,
			created_at
		FROM checklist_items
		WHERE checklist_id = $1 AND assigned_user_id IS NULL
		ORDER BY position ASC
	`

	return r.queryItems(ctx, query, checklistID)
}

func (r *ChecklistRepository) GetItemsForUser(ctx context.Context, checklistID, userID uuid.UUID) ([]model.ChecklistItem, error) {
	query := `SELECT i.id,i.checklist_id,i.stage_id,i.title,i.description,i.expected_result,
	 i.requires_review,i.estimated_minutes,COALESCE(p.position,i.position),i.created_at
	 FROM checklist_items i LEFT JOIN user_checklist_progress p ON p.checklist_item_id=i.id AND p.user_id=$2
	 WHERE i.checklist_id=$1 AND (i.assigned_user_id IS NULL OR i.assigned_user_id=$2)
	 ORDER BY COALESCE(p.position,i.position),i.id`
	return r.queryItems(ctx, query, checklistID, userID)
}

func (r *ChecklistRepository) queryItems(ctx context.Context, query string, args ...any) ([]model.ChecklistItem, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get checklist items: %w", err)
	}
	defer rows.Close()

	items := make([]model.ChecklistItem, 0)

	for rows.Next() {
		var item model.ChecklistItem
		var description sql.NullString
		var expectedResult sql.NullString
		var estimatedMinutes sql.NullInt64

		if err := rows.Scan(
			&item.ID,
			&item.ChecklistID,
			&item.StageID,
			&item.Title,
			&description,
			&expectedResult,
			&item.RequiresReview,
			&estimatedMinutes,
			&item.Position,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}

		if description.Valid {
			item.Description = description.String
		}
		if expectedResult.Valid {
			value := expectedResult.String
			item.ExpectedResult = &value
		}
		if estimatedMinutes.Valid {
			value := int(estimatedMinutes.Int64)
			item.EstimatedMinutes = &value
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checklist items: %w", err)
	}

	return items, nil
}
