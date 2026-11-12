package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

var ErrInvalidPosition = errors.New("invalid position")

type PersonalTaskRepository struct{ tx *sql.Tx }

func NewPersonalTaskRepository(tx *sql.Tx) *PersonalTaskRepository {
	return &PersonalTaskRepository{tx}
}
func (r *PersonalTaskRepository) LockUser(ctx context.Context, id uuid.UUID) error {
	var found uuid.UUID
	err := r.tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&found)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return err
}
func (r *PersonalTaskRepository) Insert(ctx context.Context, userID, checklistID uuid.UUID, position int, title, description, expected string, minutes int, review bool) (uuid.UUID, error) {
	var locked uuid.UUID
	if err := r.tx.QueryRowContext(ctx, `SELECT id FROM checklists WHERE id=$1 FOR UPDATE`, checklistID).Scan(&locked); err != nil {
		return uuid.Nil, err
	}
	rows, err := r.tx.QueryContext(ctx, `SELECT i.stage_id,p.status FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id WHERE p.user_id=$1 AND p.checklist_id=$2 ORDER BY p.position`, userID, checklistID)
	if err != nil {
		return uuid.Nil, err
	}
	defer rows.Close()
	type entry struct {
		stage  uuid.UUID
		status model.ChecklistStatus
	}
	var items []entry
	for rows.Next() {
		var x entry
		if err = rows.Scan(&x.stage, &x.status); err != nil {
			return uuid.Nil, err
		}
		items = append(items, x)
	}
	if err = rows.Err(); err != nil {
		return uuid.Nil, err
	}
	if len(items) == 0 || position < 1 || position > len(items)+1 {
		return uuid.Nil, ErrInvalidPosition
	}
	stage := items[len(items)-1].stage
	if position <= len(items) {
		stage = items[position-1].stage
	}
	status := model.StatusUnavailable
	if position == 1 || items[position-2].status == model.StatusCompleted || items[position-2].status == model.StatusVerified {
		status = model.StatusActive
	}
	if _, err = r.tx.ExecContext(ctx, `UPDATE user_checklist_progress SET position=position+1 WHERE user_id=$1 AND checklist_id=$2 AND position >= $3`, userID, checklistID, position); err != nil {
		return uuid.Nil, err
	}
	var itemID uuid.UUID
	err = r.tx.QueryRowContext(ctx, `INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,estimated_minutes,requires_review,position,assigned_user_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,(SELECT COALESCE(MAX(position),0)+1 FROM checklist_items WHERE checklist_id=$1),$8) RETURNING id`, checklistID, stage, title, description, expected, minutes, review, userID).Scan(&itemID)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = r.tx.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at)
 VALUES($1,$2,$3,$4::varchar(20),$5,CASE WHEN $4::varchar(20)='active' THEN now() ELSE NULL END)`, userID, checklistID, itemID, string(status), position)
	return itemID, err
}
