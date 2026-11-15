package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrFeedbackNotFound = errors.New("feedback checklist not found")
var ErrFeedbackIncomplete = errors.New("checklist is not fully completed and reviewed")
var ErrFeedbackExists = errors.New("feedback already submitted")
var ErrFeedbackInvalid = errors.New("invalid feedback")

type Feedback struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	ChecklistID    uuid.UUID  `json:"checklist_id"`
	Rating         int        `json:"rating"`
	Comment        string     `json:"comment"`
	DeliveryStatus string     `json:"delivery_status"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
}
type AnalyticsEvent struct {
	FeedbackID    uuid.UUID
	CorporateID   string
	ChecklistID   uuid.UUID
	ChecklistType string
	Rating        int
	Comment       string
	CreatedAt     time.Time
}
type AnalyticsReceiver interface {
	Receive(context.Context, AnalyticsEvent) error
}
type MockAnalytics struct {
	DB          *sql.DB
	Unavailable bool
}

func (m MockAnalytics) Receive(ctx context.Context, e AnalyticsEvent) error {
	if m.Unavailable {
		return errors.New("mock analytics unavailable")
	}
	_, err := m.DB.ExecContext(ctx, `INSERT INTO mock_analytics_feedback(feedback_id,corporate_id,checklist_id,checklist_type,rating,comment,created_at)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(feedback_id) DO NOTHING`, e.FeedbackID, e.CorporateID, e.ChecklistID, e.ChecklistType, e.Rating, e.Comment, e.CreatedAt)
	return err
}

type FeedbackService struct {
	db        *sql.DB
	analytics AnalyticsReceiver
}

func NewFeedbackService(db *sql.DB, analytics AnalyticsReceiver) *FeedbackService {
	return &FeedbackService{db, analytics}
}

func (s *FeedbackService) ownedChecklist(ctx context.Context, userID, checklistID uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2)`, userID, checklistID).Scan(&exists)
	return exists, err
}
func (s *FeedbackService) Get(ctx context.Context, userID, checklistID uuid.UUID) (*Feedback, error) {
	owned, err := s.ownedChecklist(ctx, userID, checklistID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrFeedbackNotFound
	}
	f := &Feedback{}
	var delivered sql.NullTime
	err = s.db.QueryRowContext(ctx, `SELECT id,user_id,checklist_id,rating,comment,delivery_status,created_at,delivered_at FROM checklist_feedback WHERE user_id=$1 AND checklist_id=$2`, userID, checklistID).Scan(&f.ID, &f.UserID, &f.ChecklistID, &f.Rating, &f.Comment, &f.DeliveryStatus, &f.CreatedAt, &delivered)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if delivered.Valid {
		f.DeliveredAt = &delivered.Time
	}
	return f, nil
}
func (s *FeedbackService) Create(ctx context.Context, userID, checklistID uuid.UUID, rating int, comment string) (*Feedback, error) {
	if rating < 1 || rating > 5 || len(comment) > 4000 {
		return nil, ErrFeedbackInvalid
	}
	comment = strings.TrimSpace(comment)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Lock the intern row to serialize concurrent submissions for this user.
	var corporateID sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT corporate_id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&corporateID); err != nil {
		return nil, err
	}
	if !corporateID.Valid {
		return nil, fmt.Errorf("corporate identity missing")
	}
	var typ string
	var total, owned, done int
	err = tx.QueryRowContext(ctx, `SELECT c.type,COUNT(i.id),COUNT(p.id),COUNT(p.id) FILTER (WHERE (i.requires_review AND p.status='verified') OR (NOT i.requires_review AND p.status IN ('completed','verified')))
 FROM checklists c JOIN checklist_items i ON i.checklist_id=c.id
 LEFT JOIN user_checklist_progress p ON p.checklist_item_id=i.id AND p.user_id=$2
		WHERE c.id=$1 AND (i.assigned_user_id IS NULL OR i.assigned_user_id=$2) AND p.hidden_at IS NULL GROUP BY c.type`, checklistID, userID).Scan(&typ, &total, &owned, &done)
	if errors.Is(err, sql.ErrNoRows) || owned == 0 {
		return nil, ErrFeedbackNotFound
	}
	if err != nil {
		return nil, err
	}
	if total == 0 || owned != total || done != total {
		return nil, ErrFeedbackIncomplete
	}
	f := &Feedback{UserID: userID, ChecklistID: checklistID, Rating: rating, Comment: comment, DeliveryStatus: "pending"}
	err = tx.QueryRowContext(ctx, `INSERT INTO checklist_feedback(user_id,checklist_id,rating,comment) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,checklist_id) DO NOTHING RETURNING id,created_at`, userID, checklistID, rating, comment).Scan(&f.ID, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFeedbackExists
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	// The database commit precedes the external delivery attempt.
	_ = s.Deliver(ctx, f.ID)
	return s.Get(ctx, userID, checklistID)
}
func (s *FeedbackService) Deliver(ctx context.Context, id uuid.UUID) error {
	var e AnalyticsEvent
	e.FeedbackID = id
	err := s.db.QueryRowContext(ctx, `SELECT u.corporate_id,f.checklist_id,c.type,f.rating,f.comment,f.created_at
 FROM checklist_feedback f JOIN users u ON u.id=f.user_id JOIN checklists c ON c.id=f.checklist_id WHERE f.id=$1 AND f.delivery_status<>'sent'`, id).Scan(&e.CorporateID, &e.ChecklistID, &e.ChecklistType, &e.Rating, &e.Comment, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.analytics.Receive(ctx, e); err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE checklist_feedback SET delivery_status='failed',delivery_error=$2 WHERE id=$1 AND delivery_status<>'sent'`, id, err.Error())
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE checklist_feedback SET delivery_status='sent',delivered_at=now(),delivery_error=NULL WHERE id=$1`, id)
	return err
}
func (s *FeedbackService) Retry(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM checklist_feedback WHERE delivery_status IN ('pending','failed') ORDER BY created_at`)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, id := range ids {
		if err = s.Deliver(ctx, id); err == nil {
			sent++
		}
	}
	return sent, nil
}
