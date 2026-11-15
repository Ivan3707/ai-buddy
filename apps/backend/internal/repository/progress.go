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
			completed_at, position, activated_at
		)
		SELECT $1, $2, $3, $4::varchar(20), $5, COALESCE(NULLIF($6,0),i.position),
			CASE WHEN $4::varchar(20) = 'active' THEN now() ELSE NULL END
		FROM checklist_items i WHERE i.id=$3
	`

	for _, item := range progress {
		result, err := r.db.ExecContext(
			ctx,
			query,
			item.UserID,
			item.ChecklistID,
			item.ChecklistItemID,
			string(item.Status),
			item.CompletedAt,
			item.Position,
		)
		if err != nil {
			return fmt.Errorf("create progress for item %s: %w", item.ChecklistItemID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrNotFound
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
			p.created_at,
			p.position,
			p.activated_at
		FROM user_checklist_progress AS p
		JOIN checklist_items AS ci
			ON ci.id = p.checklist_item_id
			AND ci.checklist_id = p.checklist_id
		JOIN checklists AS c
			ON c.id = p.checklist_id
		WHERE p.user_id = $1
		ORDER BY c.type, p.position
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get progress by user ID: %w", err)
	}
	defer rows.Close()

	progress := make([]model.UserChecklistProgress, 0)

	for rows.Next() {
		var item model.UserChecklistProgress
		var completedAt, activatedAt sql.NullTime

		err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.ChecklistID,
			&item.ChecklistItemID,
			&item.Status,
			&completedAt,
			&item.CreatedAt,
			&item.Position,
			&activatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan progress: %w", err)
		}

		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}
		if activatedAt.Valid {
			item.ActivatedAt = &activatedAt.Time
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
		SET status = $1::varchar(20), completed_at = $2,
		activated_at = CASE WHEN $1::varchar(20) = 'active' AND status <> 'active' THEN now()
		WHEN $1::varchar(20) <> 'active' THEN NULL ELSE activated_at END
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

// MakeAvailable opens an item for the intern without starting it: status 'active' with activated_at NULL.
// The web shows such a task at the top of the "Задачи" column; the intern starts it by hand (Start).
// The hint worker and the overdue report ignore it until then (they require activated_at).
func (r *ProgressRepository) MakeAvailable(ctx context.Context, userID, itemID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `UPDATE user_checklist_progress
		SET status = 'active', completed_at = NULL, activated_at = NULL
		WHERE user_id = $1 AND checklist_item_id = $2`, userID, itemID)
	if err != nil {
		return fmt.Errorf("make progress available: %w", err)
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

// Start marks an available (active, not started) item as taken into work: activated_at = now().
// It reports whether the row changed; an already started item is left as is.
func (r *ProgressRepository) Start(ctx context.Context, userID, itemID uuid.UUID) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE user_checklist_progress SET activated_at = now()
		WHERE user_id = $1 AND checklist_item_id = $2 AND status = 'active' AND activated_at IS NULL`, userID, itemID)
	if err != nil {
		return false, fmt.Errorf("start progress item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get affected rows: %w", err)
	}
	return affected > 0, nil
}

func (r *ProgressRepository) ChecklistForTrack(ctx context.Context, typ string, profession model.Profession) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type=$1 AND ($1='common' OR profession=$2)`, typ, string(profession)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

// InitializeChecklist creates missing progress rows; the first item is available (active) but not started: activated_at stays NULL.
func (r *ProgressRepository) InitializeChecklist(ctx context.Context, userID, checklistID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at)
	SELECT $1,$2,i.id,CASE WHEN i.position=(SELECT MIN(position) FROM checklist_items WHERE checklist_id=$2 AND(assigned_user_id IS NULL OR assigned_user_id=$1))AND NOT EXISTS(SELECT 1 FROM user_checklist_progress p0 WHERE p0.user_id=$1 AND p0.checklist_id=$2)THEN 'active' ELSE 'unavailable' END,
	i.position,NULL
	FROM checklist_items i WHERE i.checklist_id=$2 AND(assigned_user_id IS NULL OR assigned_user_id=$1)AND NOT EXISTS(SELECT 1 FROM user_checklist_progress p WHERE p.user_id=$1 AND p.checklist_item_id=i.id)`, userID, checklistID)
	return err
}

func (r *ProgressRepository) OwnerAndChecklist(ctx context.Context, progressID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var userID, checklistID uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT user_id,checklist_id FROM user_checklist_progress WHERE id=$1`, progressID).Scan(&userID, &checklistID)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return userID, checklistID, err
}

func (r *ProgressRepository) LockOwner(ctx context.Context, userID uuid.UUID) error {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *ProgressRepository) TrackMatchesProfession(ctx context.Context, userID, checklistID uuid.UUID) (bool, error) {
	var typ string
	var checklistProfession, userProfession sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT c.type,c.profession,u.profession FROM checklists c JOIN users u ON u.id=$1 WHERE c.id=$2`, userID, checklistID).Scan(&typ, &checklistProfession, &userProfession)
	if err != nil {
		return false, err
	}
	if typ != string(model.ChecklistTypeProfessional) {
		return true, nil
	}
	return checklistProfession.Valid && userProfession.Valid && checklistProfession.String == userProfession.String, nil
}
