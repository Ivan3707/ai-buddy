package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

var ErrFeedbackNotFound = errors.New("feedback checklist not found")
var ErrFeedbackIncomplete = errors.New("checklist is not fully completed and reviewed")
var ErrFeedbackExists = errors.New("feedback already submitted")
var ErrFeedbackInvalid = errors.New("invalid feedback")

type Feedback = model.Feedback
type AnalyticsEvent = model.AnalyticsEvent
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
	return repository.NewFeedbackRepository(m.DB).RecordAnalytics(ctx, e)
}

type FeedbackService struct {
	db        *sql.DB
	analytics AnalyticsReceiver
}

func NewFeedbackService(db *sql.DB, analytics AnalyticsReceiver) *FeedbackService {
	return &FeedbackService{db, analytics}
}

func (s *FeedbackService) ownedChecklist(ctx context.Context, userID, checklistID uuid.UUID) (bool, error) {
	return repository.NewFeedbackRepository(s.db).OwnedChecklist(ctx, userID, checklistID)
}
func (s *FeedbackService) Get(ctx context.Context, userID, checklistID uuid.UUID) (*Feedback, error) {
	owned, err := s.ownedChecklist(ctx, userID, checklistID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrFeedbackNotFound
	}
	return repository.NewFeedbackRepository(s.db).Get(ctx, userID, checklistID)
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
	repo := repository.NewFeedbackRepository(tx)
	if _, err = repo.LockCorporateID(ctx, userID); err != nil {
		return nil, err
	}
	_, total, owned, done, err := repo.Completion(ctx, userID, checklistID)
	if errors.Is(err, repository.ErrNotFound) || owned == 0 {
		return nil, ErrFeedbackNotFound
	}
	if err != nil {
		return nil, err
	}
	if total == 0 || owned != total || done != total {
		return nil, ErrFeedbackIncomplete
	}
	f, err := repo.Create(ctx, userID, checklistID, rating, comment)
	if errors.Is(err, repository.ErrConflict) {
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
	repo := repository.NewFeedbackRepository(s.db)
	e, err := repo.DeliveryEvent(ctx, id)
	if e == nil && err == nil {
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.analytics.Receive(ctx, *e); err != nil {
		_ = repo.MarkFailed(ctx, id, err.Error())
		return err
	}
	return repo.MarkSent(ctx, id)
}
func (s *FeedbackService) Retry(ctx context.Context) (int, error) {
	ids, err := repository.NewFeedbackRepository(s.db).RetryIDs(ctx)
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
