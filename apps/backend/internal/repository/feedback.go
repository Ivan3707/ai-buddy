package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type FeedbackRepository struct{ db DBTX }

func NewFeedbackRepository(db DBTX) *FeedbackRepository { return &FeedbackRepository{db: db} }
func (r *FeedbackRepository) OwnedChecklist(ctx context.Context, userID, checklistID uuid.UUID) (bool, error) {
	var v bool
	e := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2)`, userID, checklistID).Scan(&v)
	return v, e
}
func (r *FeedbackRepository) Get(ctx context.Context, userID, checklistID uuid.UUID) (*model.Feedback, error) {
	v := &model.Feedback{}
	var delivered sql.NullTime
	e := r.db.QueryRowContext(ctx, `SELECT id,user_id,checklist_id,rating,comment,delivery_status,created_at,delivered_at FROM checklist_feedback WHERE user_id=$1 AND checklist_id=$2`, userID, checklistID).Scan(&v.ID, &v.UserID, &v.ChecklistID, &v.Rating, &v.Comment, &v.DeliveryStatus, &v.CreatedAt, &delivered)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if delivered.Valid {
		v.DeliveredAt = &delivered.Time
	}
	return v, nil
}
func (r *FeedbackRepository) LockCorporateID(ctx context.Context, userID uuid.UUID) (string, error) {
	var v sql.NullString
	e := r.db.QueryRowContext(ctx, `SELECT corporate_id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&v)
	if e != nil {
		return "", e
	}
	if !v.Valid {
		return "", errors.New("corporate identity missing")
	}
	return v.String, nil
}
func (r *FeedbackRepository) Completion(ctx context.Context, userID, checklistID uuid.UUID) (typ string, total, owned, done int, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT c.type,COUNT(i.id),COUNT(p.id),COUNT(p.id) FILTER(WHERE(i.requires_review AND p.status='verified')OR(NOT i.requires_review AND p.status IN('completed','verified'))) FROM checklists c JOIN checklist_items i ON i.checklist_id=c.id LEFT JOIN user_checklist_progress p ON p.checklist_item_id=i.id AND p.user_id=$2 WHERE c.id=$1 AND(i.assigned_user_id IS NULL OR i.assigned_user_id=$2)AND p.hidden_at IS NULL GROUP BY c.type`, checklistID, userID).Scan(&typ, &total, &owned, &done)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}
func (r *FeedbackRepository) Create(ctx context.Context, userID, checklistID uuid.UUID, rating int, comment string) (*model.Feedback, error) {
	v := &model.Feedback{UserID: userID, ChecklistID: checklistID, Rating: rating, Comment: comment, DeliveryStatus: "pending"}
	e := r.db.QueryRowContext(ctx, `INSERT INTO checklist_feedback(user_id,checklist_id,rating,comment)VALUES($1,$2,$3,$4)ON CONFLICT(user_id,checklist_id)DO NOTHING RETURNING id,created_at`, userID, checklistID, rating, comment).Scan(&v.ID, &v.CreatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	return v, e
}
func (r *FeedbackRepository) DeliveryEvent(ctx context.Context, id uuid.UUID) (*model.AnalyticsEvent, error) {
	v := &model.AnalyticsEvent{FeedbackID: id}
	e := r.db.QueryRowContext(ctx, `SELECT u.corporate_id,f.checklist_id,c.type,f.rating,f.comment,f.created_at FROM checklist_feedback f JOIN users u ON u.id=f.user_id JOIN checklists c ON c.id=f.checklist_id WHERE f.id=$1 AND f.delivery_status<>'sent'`, id).Scan(&v.CorporateID, &v.ChecklistID, &v.ChecklistType, &v.Rating, &v.Comment, &v.CreatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	return v, e
}
func (r *FeedbackRepository) MarkFailed(ctx context.Context, id uuid.UUID, message string) error {
	_, e := r.db.ExecContext(ctx, `UPDATE checklist_feedback SET delivery_status='failed',delivery_error=$2 WHERE id=$1 AND delivery_status<>'sent'`, id, message)
	return e
}
func (r *FeedbackRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	_, e := r.db.ExecContext(ctx, `UPDATE checklist_feedback SET delivery_status='sent',delivered_at=now(),delivery_error=NULL WHERE id=$1`, id)
	return e
}
func (r *FeedbackRepository) RetryIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id FROM checklist_feedback WHERE delivery_status IN('pending','failed')ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (r *FeedbackRepository) RecordAnalytics(ctx context.Context, e model.AnalyticsEvent) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO mock_analytics_feedback(feedback_id,corporate_id,checklist_id,checklist_type,rating,comment,created_at)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT(feedback_id)DO NOTHING`, e.FeedbackID, e.CorporateID, e.ChecklistID, e.ChecklistType, e.Rating, e.Comment, e.CreatedAt)
	return err
}
