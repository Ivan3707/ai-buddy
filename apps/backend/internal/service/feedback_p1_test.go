package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestFeedbackConcurrentCreateAndRetryHasOneStoredEvent(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	userID := uuid.New()
	corporateID := "p1-feedback-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,role,profession,corporate_id) VALUES($1,$2,'hash','intern','backend',$3)`, userID, corporateID+"@example.test", corporateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID); err != nil {
			t.Errorf("cleanup feedback user: %v", err)
		}
	})
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM mock_analytics_feedback WHERE corporate_id=$1`, corporateID); err != nil {
			t.Errorf("cleanup analytics events: %v", err)
		}
		if _, err := db.ExecContext(context.Background(), `DELETE FROM checklist_feedback WHERE user_id=$1`, userID); err != nil {
			t.Errorf("cleanup feedback: %v", err)
		}
	})
	for _, entry := range []struct{ checklist, item uuid.UUID }{{fixture.commonID, fixture.commonItems[0].ID}, {fixture.professionalID, fixture.profItems[0].ID}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,completed_at) VALUES($1,$2,$3,'completed',1,now())`, userID, entry.checklist, entry.item); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewFeedbackService(db, MockAnalytics{DB: db})
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := svc.Create(ctx, userID, fixture.commonID, 5, "ok")
			results <- err
		}()
	}
	close(start)
	success, duplicate := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrFeedbackExists):
			duplicate++
		default:
			t.Fatalf("concurrent feedback create: %v", err)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("create results: success=%d duplicate=%d", success, duplicate)
	}
	var commonID uuid.UUID
	var status string
	if err := db.QueryRowContext(ctx, `SELECT id,delivery_status FROM checklist_feedback WHERE user_id=$1 AND checklist_id=$2`, userID, fixture.commonID).Scan(&commonID, &status); err != nil || status != "sent" {
		t.Fatalf("common feedback: id=%s status=%s err=%v", commonID, status, err)
	}
	failed := NewFeedbackService(db, MockAnalytics{DB: db, Unavailable: true})
	professional, err := failed.Create(ctx, userID, fixture.professionalID, 4, "retry")
	if err != nil || professional.DeliveryStatus != "failed" {
		t.Fatalf("failed delivery setup: %+v err=%v", professional, err)
	}
	start = make(chan struct{})
	retries := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := svc.Retry(ctx)
			retries <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-retries; err != nil {
			t.Fatalf("concurrent retry: %v", err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT delivery_status FROM checklist_feedback WHERE id=$1`, professional.ID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("retry status=%s err=%v", status, err)
	}
	for _, id := range []uuid.UUID{commonID, professional.ID} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mock_analytics_feedback WHERE feedback_id=$1`, id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("analytics duplicates for %s: count=%d err=%v", id, count, err)
		}
	}
}
