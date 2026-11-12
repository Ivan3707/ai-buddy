package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type Hint struct {
	ID        uuid.UUID  `json:"id"`
	ItemID    uuid.UUID  `json:"item_id"`
	Text      string     `json:"text"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}
type DueHint struct {
	ID                           uuid.UUID
	Title, Description, Expected string
	Minutes                      int
}
type HintRepository struct{ db *sql.DB }

func NewHintRepository(db *sql.DB) *HintRepository { return &HintRepository{db} }
func (r *HintRepository) EnqueueDue(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO task_hints(progress_id)
 SELECT p.id FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id
 WHERE p.status='active' AND p.activated_at IS NOT NULL AND i.estimated_minutes IS NOT NULL
 AND p.activated_at + i.estimated_minutes * interval '1 minute' < now()
 ON CONFLICT(progress_id) DO NOTHING`)
	return err
}
func (r *HintRepository) Claim(ctx context.Context) (*DueHint, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `WITH due AS (
 SELECT h.id FROM task_hints h JOIN user_checklist_progress p ON p.id=h.progress_id
 WHERE p.status='active' AND ((h.status='pending' AND h.next_attempt_at<=now()) OR
 (h.status='processing' AND h.next_attempt_at<now()-interval '2 minutes'))
 ORDER BY h.next_attempt_at FOR UPDATE OF h SKIP LOCKED LIMIT 1)
 UPDATE task_hints h SET status='processing',next_attempt_at=now() FROM due,
 user_checklist_progress p,checklist_items i
 WHERE h.id=due.id AND p.id=h.progress_id AND i.id=p.checklist_item_id
 RETURNING h.id,i.title,i.description,COALESCE(i.expected_result,''),i.estimated_minutes`)
	var hint DueHint
	err = row.Scan(&hint.ID, &hint.Title, &hint.Description, &hint.Expected, &hint.Minutes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &hint, nil
}
func (r *HintRepository) Resolve(ctx context.Context, id uuid.UUID, text string, success bool) error {
	if success {
		_, err := r.db.ExecContext(ctx, `UPDATE task_hints SET text=$1,status='ready',attempts=attempts+1 WHERE id=$2 AND status='processing'`, text, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE task_hints SET attempts=attempts+1,
 status=CASE WHEN attempts>=2 THEN 'ready' ELSE 'pending' END,
 text=CASE WHEN attempts>=2 THEN $1 ELSE text END,
 next_attempt_at=now()+interval '1 minute' WHERE id=$2 AND status='processing'`, text, id)
	return err
}
func (r *HintRepository) List(ctx context.Context, userID uuid.UUID) ([]Hint, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT h.id,p.checklist_item_id,h.text,h.read_at,h.created_at FROM task_hints h
 JOIN user_checklist_progress p ON p.id=h.progress_id WHERE p.user_id=$1 AND h.status='ready' ORDER BY h.created_at,h.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Hint, 0)
	for rows.Next() {
		var h Hint
		var read sql.NullTime
		if err = rows.Scan(&h.ID, &h.ItemID, &h.Text, &read, &h.CreatedAt); err != nil {
			return nil, err
		}
		if read.Valid {
			h.ReadAt = &read.Time
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (r *HintRepository) GetByItem(ctx context.Context, userID, itemID uuid.UUID) (*Hint, error) {
	var h Hint
	var read sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT h.id,p.checklist_item_id,h.text,h.read_at,h.created_at FROM task_hints h
 JOIN user_checklist_progress p ON p.id=h.progress_id WHERE p.user_id=$1 AND p.checklist_item_id=$2 AND h.status='ready'`, userID, itemID).Scan(&h.ID, &h.ItemID, &h.Text, &read, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if read.Valid {
		h.ReadAt = &read.Time
	}
	return &h, nil
}
func (r *HintRepository) MarkRead(ctx context.Context, userID, hintID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `UPDATE task_hints h SET read_at=COALESCE(read_at,now()) FROM user_checklist_progress p
 WHERE h.id=$1 AND p.id=h.progress_id AND p.user_id=$2 AND h.status='ready'`, hintID, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
