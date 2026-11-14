package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type timeoutAnalytics struct{}

func (timeoutAnalytics) Receive(ctx context.Context, _ AnalyticsEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestFeedbackPostgresDelivery(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	id := uuid.New()
	corp := "feedback-test-" + uuid.NewString()
	_, err := db.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,role,profession,corporate_id) VALUES($1,$2,'test-hash','intern','backend',$3)`, id, corp+"@example.test", corp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, e := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id); e != nil {
			t.Errorf("cleanup user: %v", e)
		}
	})
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM mock_analytics_feedback WHERE corporate_id=$1`, corp)
		if _, e := db.ExecContext(context.Background(), `DELETE FROM checklist_feedback WHERE user_id=$1`, id); e != nil {
			t.Errorf("cleanup feedback: %v", e)
		}
	})
	for _, item := range []struct{ checklist, item uuid.UUID }{{fixture.commonID, fixture.commonItems[0].ID}, {fixture.professionalID, fixture.profItems[0].ID}} {
		if _, err = db.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at) VALUES($1,$2,$3,'active',1,now())`, id, item.checklist, item.item); err != nil {
			t.Fatal(err)
		}
	}
	sender := NewFeedbackService(db, MockAnalytics{DB: db, Unavailable: true})
	if _, err = sender.Create(ctx, id, fixture.commonID, 5, ""); !errors.Is(err, ErrFeedbackIncomplete) {
		t.Fatalf("incomplete: %v", err)
	}
	if _, err = sender.Create(ctx, id, fixture.professionalID, 0, ""); !errors.Is(err, ErrFeedbackInvalid) {
		t.Fatalf("invalid rating: %v", err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE checklist_items SET requires_review=true WHERE id=$1`, fixture.commonItems[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE user_checklist_progress SET status='completed',completed_at=now(),activated_at=NULL WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = sender.Create(ctx, id, fixture.commonID, 5, ""); !errors.Is(err, ErrFeedbackIncomplete) {
		t.Fatalf("pending Buddy review: %v", err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE user_checklist_progress SET status='verified' WHERE user_id=$1 AND checklist_id=$2`, id, fixture.commonID); err != nil {
		t.Fatal(err)
	}
	feedback, err := sender.Create(ctx, id, fixture.commonID, 5, "  useful  ")
	if err != nil {
		t.Fatal(err)
	}
	if feedback.DeliveryStatus != "failed" || feedback.Comment != "useful" {
		t.Fatalf("stored feedback: %+v", feedback)
	}
	if _, err = sender.Create(ctx, id, fixture.commonID, 4, ""); !errors.Is(err, ErrFeedbackExists) {
		t.Fatalf("duplicate: %v", err)
	}
	other, err := sender.Get(ctx, id, fixture.professionalID)
	if err != nil || other != nil {
		t.Fatalf("other checklist feedback: %+v %v", other, err)
	}
	sender.analytics = timeoutAnalytics{}
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	if err = sender.Deliver(timeoutCtx, feedback.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("analytics timeout: %v", err)
	}
	cancel()
	var retryable string
	if err = db.QueryRowContext(ctx, `SELECT delivery_status FROM checklist_feedback WHERE id=$1`, feedback.ID).Scan(&retryable); err != nil || retryable != "failed" {
		t.Fatalf("timed out feedback not retryable: status=%s err=%v", retryable, err)
	}
	sender.analytics = MockAnalytics{DB: db}
	sent, err := sender.Retry(ctx)
	if err != nil || sent != 1 {
		t.Fatalf("retry: %d %v", sent, err)
	}
	sent, err = sender.Retry(ctx)
	if err != nil || sent != 0 {
		t.Fatalf("idempotent retry: %d %v", sent, err)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_analytics_feedback WHERE feedback_id=$1`, feedback.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("mock events: %d %v", count, err)
	}
}
