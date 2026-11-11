package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type ProgressRepository struct {
	db DBTX
}

func NewProgressRepository(db DBTX) *ProgressRepository {
	return &ProgressRepository{db: db}
}

func (r *ProgressRepository) CreateMany(
	ctx context.Context,
	progress []model.UserChecklistProgress,
) error {
	query := `
		INSERT INTO user_checklist_progress (
			user_id,
			checklist_id,
			checklist_item_id,
			status,
			completed_at
		)
		VALUES ($1, $2, $3, $4, $5)
	`

	for _, item := range progress {
		_, err := r.db.ExecContext(
			ctx,
			query,
			item.UserID,
			item.ChecklistID,
			item.ChecklistItemID,
			string(item.Status),
			item.CompletedAt,
		)
		if err != nil {
			return fmt.Errorf("create progress for item %s: %w", item.ChecklistItemID, err)
		}
	}

	return nil
}

func (r *ProgressRepository) GetByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]model.UserChecklistProgress, error) {
	query := `
		SELECT
			p.id,
			p.user_id,
			p.checklist_id,
			p.checklist_item_id,
			p.status,
			p.completed_at,
			p.created_at
		FROM user_checklist_progress AS p
		JOIN checklist_items AS ci
			ON ci.id = p.checklist_item_id
			AND ci.checklist_id = p.checklist_id
		JOIN checklists AS c
			ON c.id = p.checklist_id
		WHERE p.user_id = $1
		ORDER BY c.type, ci.position
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get progress by user ID: %w", err)
	}
	defer rows.Close()

	progress := make([]model.UserChecklistProgress, 0)

	for rows.Next() {
		var item model.UserChecklistProgress
		var completedAt sql.NullTime

		err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.ChecklistID,
			&item.ChecklistItemID,
			&item.Status,
			&completedAt,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan progress: %w", err)
		}

		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}

		progress = append(progress, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate progress: %w", err)
	}

	return progress, nil
}

func (r *ProgressRepository) GetByUserAndItem(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
) (*model.UserChecklistProgress, error) {
	query := `
		SELECT
			id,
			user_id,
			checklist_id,
			checklist_item_id,
			status,
			completed_at,
			created_at
		FROM user_checklist_progress
		WHERE user_id = $1 AND checklist_item_id = $2
		FOR UPDATE
	`

	item := &model.UserChecklistProgress{}
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, userID, itemID).Scan(
		&item.ID,
		&item.UserID,
		&item.ChecklistID,
		&item.ChecklistItemID,
		&item.Status,
		&completedAt,
		&item.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get progress by user and item: %w", err)
	}

	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}

	return item, nil
}

// GetByUserAndItemRead returns progress without taking a row lock. It is used
// by read-only service methods; state transitions must use GetByUserAndItem.
func (r *ProgressRepository) GetByUserAndItemRead(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
) (*model.UserChecklistProgress, error) {
	query := `
		SELECT
			id,
			user_id,
			checklist_id,
			checklist_item_id,
			status,
			completed_at,
			created_at
		FROM user_checklist_progress
		WHERE user_id = $1 AND checklist_item_id = $2
	`

	item := &model.UserChecklistProgress{}
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, userID, itemID).Scan(
		&item.ID,
		&item.UserID,
		&item.ChecklistID,
		&item.ChecklistItemID,
		&item.Status,
		&completedAt,
		&item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get progress by user and item for reading: %w", err)
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}

	return item, nil
}

// GetByIDForUpdate locks a progress row inside the caller's transaction.
// The repository does not start or commit transactions itself.
func (r *ProgressRepository) GetByIDForUpdate(
	ctx context.Context,
	id uuid.UUID,
) (*model.UserChecklistProgress, error) {
	query := `
		SELECT
			id,
			user_id,
			checklist_id,
			checklist_item_id,
			status,
			completed_at,
			created_at
		FROM user_checklist_progress
		WHERE id = $1
		FOR UPDATE
	`

	item := &model.UserChecklistProgress{}
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID,
		&item.UserID,
		&item.ChecklistID,
		&item.ChecklistItemID,
		&item.Status,
		&completedAt,
		&item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get progress by ID for update: %w", err)
	}

	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}

	return item, nil
}

func (r *ProgressRepository) UpdateStatus(
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
	status model.ChecklistStatus,
	completedAt *time.Time,
) error {
	query := `
		UPDATE user_checklist_progress
		SET status = $1, completed_at = $2
		WHERE user_id = $3 AND checklist_item_id = $4
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		string(status),
		completedAt,
		userID,
		itemID,
	)
	if err != nil {
		return fmt.Errorf("update progress status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}
