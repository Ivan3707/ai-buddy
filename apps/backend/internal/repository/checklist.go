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

func (r *ChecklistRepository) GetItems(
	ctx context.Context,
	checklistID uuid.UUID,
) ([]model.ChecklistItem, error) {
	query := `
		SELECT id, checklist_id, title, description, position, created_at
		FROM checklist_items
		WHERE checklist_id = $1
		ORDER BY position ASC
	`

	rows, err := r.db.QueryContext(ctx, query, checklistID)
	if err != nil {
		return nil, fmt.Errorf("get checklist items: %w", err)
	}
	defer rows.Close()

	items := make([]model.ChecklistItem, 0)

	for rows.Next() {
		var item model.ChecklistItem

		err := rows.Scan(
			&item.ID,
			&item.ChecklistID,
			&item.Title,
			&item.Description,
			&item.Position,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checklist items: %w", err)
	}

	return items, nil
}
