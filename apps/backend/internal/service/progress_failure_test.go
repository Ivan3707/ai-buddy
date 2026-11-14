package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestReviewSubmissionRejectsChangedProgressWithoutReview(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	svc := NewProgressService(db)
	text := "work"
	submission, err := svc.SubmitItem(ctx, user.ID, itemID, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE user_checklist_progress SET status='active', completed_at=NULL WHERE user_id=$1 AND checklist_item_id=$2`, user.ID, itemID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil); !errors.Is(err, ErrReviewNotPending) {
		t.Fatalf("changed progress: %v", err)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, itemID)
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, progress.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Status != model.StatusActive || len(history) != 1 || history[0].Status != model.SubmissionPending || history[0].ReviewedAt != nil {
		t.Fatalf("refusal changed progress/submission: status=%s history=%+v", progress.Status, history)
	}
}

func TestReviewSubmissionRollsBackWhenProgressUpdateFails(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	svc := NewProgressService(db)
	text := "work"
	submission, err := svc.SubmitItem(ctx, user.ID, itemID, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, itemID)
	suffix := "test_review_" + uuid.NewString()[:8]
	function := suffix + "_fn"
	create := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id = '%s'::uuid THEN RAISE EXCEPTION 'forced progress update failure'; END IF; RETURN NEW; END $$`, function, progress.ID)
	if _, err := db.ExecContext(ctx, create); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON user_checklist_progress`, suffix)); err != nil {
			t.Errorf("drop test trigger: %v", err)
		}
		if _, err := db.ExecContext(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, function)); err != nil {
			t.Errorf("drop test function: %v", err)
		}
	})
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON user_checklist_progress FOR EACH ROW EXECUTE FUNCTION %s()`, suffix, function)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil); err == nil {
		t.Fatal("expected progress update failure")
	}
	progress = getProgressForTest(t, db, ctx, user.ID, itemID)
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, progress.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Status != model.StatusCompleted || len(history) != 1 || history[0].Status != model.SubmissionPending || history[0].ReviewedAt != nil {
		t.Fatalf("review did not roll back atomically: status=%s history=%+v", progress.Status, history)
	}
}
