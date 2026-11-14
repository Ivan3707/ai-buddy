package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func setRequiresReview(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	itemIDs ...uuid.UUID,
) {
	t.Helper()
	for _, itemID := range itemIDs {
		if _, err := db.ExecContext(ctx, `
			UPDATE checklist_items SET requires_review = TRUE WHERE id = $1
		`, itemID); err != nil {
			t.Fatalf("mark item %s as requiring review: %v", itemID, err)
		}
	}
}

func getProgressForTest(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	userID uuid.UUID,
	itemID uuid.UUID,
) *model.UserChecklistProgress {
	t.Helper()
	progress, err := repository.NewProgressRepository(db).GetByUserAndItem(
		ctx,
		userID,
		itemID,
	)
	if err != nil {
		t.Fatalf("get progress for item %s: %v", itemID, err)
	}
	return progress
}

func TestProgressService_OldProfessionTrackIsReadOnly(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 2)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.profItems[0].ID, fixture.profItems[1].ID)
	svc := NewProgressService(db)
	text := "submitted work"
	submission, err := svc.SubmitItem(ctx, user.ID, fixture.profItems[0].ID, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET profession=$1 WHERE id=$2`, model.ProfessionFrontend, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteItem(ctx, user.ID, fixture.profItems[1].ID); !errors.Is(err, ErrOutdatedProfessionTrack) {
		t.Fatalf("complete old track: %v", err)
	}
	if _, err := svc.SubmitItem(ctx, user.ID, fixture.profItems[1].ID, &text, nil); !errors.Is(err, ErrOutdatedProfessionTrack) {
		t.Fatalf("submit old track: %v", err)
	}
	if err := svc.ReviewSubmission(ctx, uuid.New(), submission.ID, true, nil); !errors.Is(err, ErrOutdatedProfessionTrack) {
		t.Fatalf("review old track: %v", err)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, fixture.profItems[0].ID)
	if progress.Status != model.StatusCompleted {
		t.Fatalf("historical progress changed: %s", progress.Status)
	}
	if err := svc.CompleteItem(ctx, user.ID, fixture.commonItems[0].ID); err != nil {
		t.Fatalf("common checklist should remain independent: %v", err)
	}
}

func TestProgressService_CompleteItem_Success(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		3,
		2,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	svc := NewProgressService(db)

	err := svc.CompleteItem(ctx, user.ID, fixture.commonItems[0].ID)
	if err != nil {
		t.Fatalf("complete item: %v", err)
	}

	progress, err := repository.NewProgressRepository(db).GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get progress: %v", err)
	}

	progressByItemID := make(map[uuid.UUID]model.UserChecklistProgress)
	for _, item := range progress {
		progressByItemID[item.ChecklistItemID] = item
	}

	// Первое общее задание завершено.
	completed := progressByItemID[fixture.commonItems[0].ID]
	if completed.Status != model.StatusCompleted {
		t.Errorf("expected completed status, got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}

	// Второе общее задание активировано.
	next := progressByItemID[fixture.commonItems[1].ID]
	if next.Status != model.StatusActive {
		t.Errorf("expected next item to be active, got %q", next.Status)
	}
	if next.CompletedAt != nil {
		t.Error("expected next item CompletedAt to be nil")
	}

	// Профессиональный чек-лист не затронут.
	professional := progressByItemID[fixture.profItems[0].ID]
	if professional.Status != model.StatusActive {
		t.Errorf(
			"expected professional item to remain active, got %q",
			professional.Status,
		)
	}
}

func TestProgressService_CompleteItem_AlreadyCompleted(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		2,
		1,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	svc := NewProgressService(db)
	itemID := fixture.commonItems[0].ID

	if err := svc.CompleteItem(ctx, user.ID, itemID); err != nil {
		t.Fatalf("first completion: %v", err)
	}

	progressRepo := repository.NewProgressRepository(db)

	before, err := progressRepo.GetByUserAndItem(ctx, user.ID, itemID)
	if err != nil {
		t.Fatalf("get progress before repeated completion: %v", err)
	}

	if err := svc.CompleteItem(ctx, user.ID, itemID); err != nil {
		t.Fatalf("repeated completion: %v", err)
	}

	after, err := progressRepo.GetByUserAndItem(ctx, user.ID, itemID)
	if err != nil {
		t.Fatalf("get progress after repeated completion: %v", err)
	}

	if after.Status != model.StatusCompleted {
		t.Errorf("expected completed status, got %q", after.Status)
	}

	if before.CompletedAt == nil || after.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}

	if !before.CompletedAt.Equal(*after.CompletedAt) {
		t.Error("repeated completion changed CompletedAt")
	}

	next, err := progressRepo.GetByUserAndItem(
		ctx,
		user.ID,
		fixture.commonItems[1].ID,
	)
	if err != nil {
		t.Fatalf("get next item progress: %v", err)
	}

	if next.Status != model.StatusActive {
		t.Errorf("expected next item to remain active, got %q", next.Status)
	}
}

func TestProgressService_CompleteItem_NotActive(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		2,
		1,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	err := NewProgressService(db).CompleteItem(
		ctx,
		user.ID,
		fixture.commonItems[1].ID,
	)
	if !errors.Is(err, ErrItemNotActive) {
		t.Fatalf("expected ErrItemNotActive, got %v", err)
	}

	progress, err := repository.NewProgressRepository(db).GetByUserAndItem(
		ctx,
		user.ID,
		fixture.commonItems[0].ID,
	)
	if err != nil {
		t.Fatalf("get first item progress: %v", err)
	}

	if progress.Status != model.StatusActive {
		t.Errorf("expected first item to remain active, got %q", progress.Status)
	}
}

func TestProgressService_CompleteItem_LastItem(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		3,
		1,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	svc := NewProgressService(db)

	for _, item := range fixture.commonItems {
		if err := svc.CompleteItem(ctx, user.ID, item.ID); err != nil {
			t.Fatalf("complete item %s: %v", item.ID, err)
		}
	}

	progress, err := repository.NewProgressRepository(db).GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get progress: %v", err)
	}

	for _, item := range progress {
		if item.ChecklistID != fixture.commonID {
			continue
		}

		if item.Status != model.StatusCompleted {
			t.Errorf(
				"expected all common items to be completed, item %s has status %q",
				item.ChecklistItemID,
				item.Status,
			)
		}

		if item.CompletedAt == nil {
			t.Errorf("expected CompletedAt for item %s", item.ChecklistItemID)
		}
	}
}

func TestProgressService_CompleteItem_RollbackOnMissingNextProgress(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		2,
		1,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	// Имитируем повреждённое состояние: у следующего задания нет прогресса.
	_, err := db.ExecContext(
		ctx,
		`DELETE FROM user_checklist_progress
		 WHERE user_id = $1 AND checklist_item_id = $2`,
		user.ID,
		fixture.commonItems[1].ID,
	)
	if err != nil {
		t.Fatalf("delete next item progress: %v", err)
	}

	err = NewProgressService(db).CompleteItem(
		ctx,
		user.ID,
		fixture.commonItems[0].ID,
	)
	if !errors.Is(err, ErrProgressIncomplete) {
		t.Fatalf("expected ErrProgressIncomplete, got %v", err)
	}

	// Текущее задание не должно завершиться из-за rollback.
	current, err := repository.NewProgressRepository(db).GetByUserAndItem(
		ctx,
		user.ID,
		fixture.commonItems[0].ID,
	)
	if err != nil {
		t.Fatalf("get current item progress: %v", err)
	}

	if current.Status != model.StatusActive {
		t.Errorf("expected current item to remain active, got %q", current.Status)
	}

	if current.CompletedAt != nil {
		t.Error("expected CompletedAt to remain nil after rollback")
	}
}

func TestProgressService_CompleteItem_NotFound(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	err := NewProgressService(db).CompleteItem(
		ctx,
		uuid.New(),
		uuid.New(),
	)

	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected repository.ErrNotFound, got %v", err)
	}
}

func TestProgressService_CompleteItem_RequiresSubmission(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.commonItems[0].ID)

	err := NewProgressService(db).CompleteItem(ctx, user.ID, fixture.commonItems[0].ID)
	if !errors.Is(err, ErrSubmissionRequired) {
		t.Fatalf("expected ErrSubmissionRequired, got %v", err)
	}
	if got := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[0].ID); got.Status != model.StatusActive {
		t.Errorf("expected review item to remain active, got %q", got.Status)
	}
}

func TestProgressService_SubmitItem_ResultVariantsAndNextItem(t *testing.T) {
	tests := []struct {
		name string
		text *string
		url  *string
	}{
		{name: "text", text: stringPointer("done")},
		{name: "url", url: stringPointer("https://example.com/result")},
		{name: "text and url", text: stringPointer("done"), url: stringPointer("https://example.com/result")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx := openChecklistTestDB(t)
			fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
			user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
			itemID := fixture.commonItems[0].ID
			setRequiresReview(t, db, ctx, itemID)

			submission, err := NewProgressService(db).SubmitItem(ctx, user.ID, itemID, tt.text, tt.url)
			if err != nil {
				t.Fatalf("submit item: %v", err)
			}
			if submission.Status != model.SubmissionPending || submission.AttemptNumber != 1 {
				t.Errorf("unexpected submission state: status=%q attempt=%d", submission.Status, submission.AttemptNumber)
			}
			if !equalOptionalString(submission.ResultText, tt.text) || !equalOptionalString(submission.ResultURL, tt.url) {
				t.Errorf("stored result differs from submitted result")
			}

			current := getProgressForTest(t, db, ctx, user.ID, itemID)
			if current.Status != model.StatusCompleted || current.CompletedAt == nil {
				t.Errorf("expected completed progress with timestamp, got status=%q timestamp=%v", current.Status, current.CompletedAt)
			}
			next := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[1].ID)
			if next.Status != model.StatusActive || next.CompletedAt != nil {
				t.Errorf("expected next item active without timestamp, got status=%q timestamp=%v", next.Status, next.CompletedAt)
			}
		})
	}
}

func TestProgressService_SubmitItem_Validation(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	svc := NewProgressService(db)

	blank := "  \t"
	if _, err := svc.SubmitItem(ctx, user.ID, fixture.commonItems[0].ID, &blank, nil); !errors.Is(err, ErrInvalidSubmission) {
		t.Errorf("empty result: expected ErrInvalidSubmission, got %v", err)
	}

	text := "done"
	if _, err := svc.SubmitItem(ctx, user.ID, fixture.commonItems[0].ID, &text, nil); !errors.Is(err, ErrInvalidSubmission) {
		t.Errorf("self-reviewed item: expected ErrInvalidSubmission, got %v", err)
	}

	other := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.commonItems[0].ID)
	if _, err := svc.SubmitItem(ctx, other.ID, fixture.commonItems[1].ID, &text, nil); !errors.Is(err, ErrItemNotActive) {
		t.Errorf("foreign/unavailable item: expected ErrItemNotActive, got %v", err)
	}
	if _, err := svc.SubmitItem(ctx, uuid.New(), fixture.commonItems[0].ID, &text, nil); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("foreign user: expected repository.ErrNotFound, got %v", err)
	}
	if _, err := svc.SubmitItem(ctx, user.ID, uuid.New(), &text, nil); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("foreign item: expected repository.ErrNotFound, got %v", err)
	}
}

func TestProgressService_ReviewSubmission_Approve(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.commonItems[0].ID)
	svc := NewProgressService(db)
	text := "result"
	submission, err := svc.SubmitItem(ctx, user.ID, fixture.commonItems[0].ID, &text, nil)
	if err != nil {
		t.Fatalf("submit item: %v", err)
	}
	before := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[0].ID)

	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil); err != nil {
		t.Fatalf("approve submission: %v", err)
	}
	after := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[0].ID)
	if after.Status != model.StatusVerified || after.CompletedAt == nil || !after.CompletedAt.Equal(*before.CompletedAt) {
		t.Errorf("expected verified progress preserving completed_at, got status=%q timestamp=%v", after.Status, after.CompletedAt)
	}
	stored, err := repository.NewSubmissionRepository(db).GetByID(ctx, submission.ID)
	if err != nil {
		t.Fatalf("get reviewed submission: %v", err)
	}
	if stored.Status != model.SubmissionApproved || stored.ReviewedAt == nil || stored.ReviewedBy == nil || *stored.ReviewedBy != *user.BuddyID {
		t.Errorf("unexpected approved submission: %+v", stored)
	}
}

func TestProgressService_ReviewSubmission_RejectRetryAndHistory(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	svc := NewProgressService(db)
	firstText := "first immutable result"
	first, err := svc.SubmitItem(ctx, user.ID, itemID, &firstText, nil)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}

	comment := "please revise"
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, first.ID, false, &comment); err != nil {
		t.Fatalf("reject submission: %v", err)
	}
	current := getProgressForTest(t, db, ctx, user.ID, itemID)
	if current.Status != model.StatusActive || current.CompletedAt != nil {
		t.Errorf("expected rejected item active without timestamp, got status=%q timestamp=%v", current.Status, current.CompletedAt)
	}
	next := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[1].ID)
	if next.Status != model.StatusActive {
		t.Errorf("expected already opened next item to remain active, got %q", next.Status)
	}

	secondText := "second result"
	second, err := svc.SubmitItem(ctx, user.ID, itemID, &secondText, nil)
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if second.AttemptNumber != 2 {
		t.Errorf("expected attempt 2, got %d", second.AttemptNumber)
	}
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, current.ID)
	if err != nil {
		t.Fatalf("list submission history: %v", err)
	}
	if len(history) != 2 || history[0].Status != model.SubmissionRejected || history[1].Status != model.SubmissionPending {
		t.Fatalf("unexpected history: %+v", history)
	}
	if history[0].ResultText == nil || *history[0].ResultText != firstText || history[0].ReviewComment == nil || *history[0].ReviewComment != comment {
		t.Errorf("first submission history was not preserved: %+v", history[0])
	}
}

func TestProgressService_ReviewSubmission_AuthorizationAndConflicts(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.commonItems[0].ID)
	svc := NewProgressService(db)
	text := "result"
	submission, err := svc.SubmitItem(ctx, user.ID, fixture.commonItems[0].ID, &text, nil)
	if err != nil {
		t.Fatalf("submit item: %v", err)
	}

	foreign := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	if err := svc.ReviewSubmission(ctx, *foreign.BuddyID, submission.ID, true, nil); !errors.Is(err, ErrInternNotAssigned) {
		t.Errorf("foreign buddy: expected ErrInternNotAssigned, got %v", err)
	}
	if err := svc.ReviewSubmission(ctx, foreign.ID, submission.ID, true, nil); !errors.Is(err, ErrUserNotBuddy) {
		t.Errorf("intern reviewer: expected ErrUserNotBuddy, got %v", err)
	}
	blank := "   "
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, false, &blank); !errors.Is(err, ErrInvalidSubmission) {
		t.Errorf("blank reject comment: expected ErrInvalidSubmission, got %v", err)
	}
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil); err != nil {
		t.Fatalf("approve submission: %v", err)
	}
	if err := svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil); !errors.Is(err, ErrSubmissionAlreadyReviewed) {
		t.Errorf("repeated review: expected ErrSubmissionAlreadyReviewed, got %v", err)
	}
}

func TestProgressService_SubmitItem_Concurrent(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	svc := NewProgressService(db)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			text := "result"
			_, err := svc.SubmitItem(ctx, user.ID, itemID, &text, nil)
			results <- err
		}()
	}
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		} else if !errors.Is(err, ErrItemNotActive) && !errors.Is(err, ErrInvalidSubmission) {
			t.Errorf("unexpected concurrent submit error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("expected one successful submit, got %d", successes)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, itemID)
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, progress.ID)
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(history) != 1 || history[0].AttemptNumber != 1 {
		t.Errorf("expected one attempt numbered 1, got %+v", history)
	}
}

func TestProgressService_ReviewSubmission_Concurrent(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	setRequiresReview(t, db, ctx, fixture.commonItems[0].ID)
	svc := NewProgressService(db)
	text := "result"
	submission, err := svc.SubmitItem(ctx, user.ID, fixture.commonItems[0].ID, &text, nil)
	if err != nil {
		t.Fatalf("submit item: %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil)
		}()
	}
	close(start)
	successes := 0
	conflicts := 0
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSubmissionAlreadyReviewed):
			conflicts++
		default:
			t.Errorf("unexpected concurrent review error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Errorf("expected one success and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestProgressService_SubmitItem_RollbackOnMissingNextProgress(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	if _, err := db.ExecContext(ctx, `
		DELETE FROM user_checklist_progress WHERE user_id = $1 AND checklist_item_id = $2
	`, user.ID, fixture.commonItems[1].ID); err != nil {
		t.Fatalf("delete next progress: %v", err)
	}
	text := "result"
	_, err := NewProgressService(db).SubmitItem(ctx, user.ID, itemID, &text, nil)
	if !errors.Is(err, ErrProgressIncomplete) {
		t.Fatalf("expected ErrProgressIncomplete, got %v", err)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, itemID)
	if progress.Status != model.StatusActive || progress.CompletedAt != nil {
		t.Errorf("expected progress rollback, got status=%q timestamp=%v", progress.Status, progress.CompletedAt)
	}
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, progress.ID)
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected submission rollback, got %d rows", len(history))
	}
}

func TestProgressService_SubmitOldItem_PreservesLaterCompletedOrVerified(t *testing.T) {
	for _, status := range []model.ChecklistStatus{model.StatusCompleted, model.StatusVerified} {
		t.Run(string(status), func(t *testing.T) {
			db, ctx := openChecklistTestDB(t)
			fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
			user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
			firstID := fixture.commonItems[0].ID
			secondID := fixture.commonItems[1].ID
			setRequiresReview(t, db, ctx, firstID)
			svc := NewProgressService(db)
			text := "first"
			first, err := svc.SubmitItem(ctx, user.ID, firstID, &text, nil)
			if err != nil {
				t.Fatalf("submit first item: %v", err)
			}
			comment := "revise"
			if err := svc.ReviewSubmission(ctx, *user.BuddyID, first.ID, false, &comment); err != nil {
				t.Fatalf("reject first item: %v", err)
			}
			if _, err := db.ExecContext(ctx, `
				UPDATE user_checklist_progress
				SET status = $1, completed_at = NOW()
				WHERE user_id = $2 AND checklist_item_id = $3
			`, string(status), user.ID, secondID); err != nil {
				t.Fatalf("set later progress status: %v", err)
			}
			text = "retry"
			if _, err := svc.SubmitItem(ctx, user.ID, firstID, &text, nil); err != nil {
				t.Fatalf("resubmit old item: %v", err)
			}
			later := getProgressForTest(t, db, ctx, user.ID, secondID)
			if later.Status != status || later.CompletedAt == nil {
				t.Errorf("later progress changed: status=%q timestamp=%v", later.Status, later.CompletedAt)
			}
		})
	}
}

func TestProgressService_SubmissionReadsAreScoped(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	firstIntern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	secondIntern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	itemID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	svc := NewProgressService(db)

	firstText, secondText := "first buddy", "second buddy"
	firstSubmission, err := svc.SubmitItem(ctx, firstIntern.ID, itemID, &firstText, nil)
	if err != nil {
		t.Fatalf("submit first intern result: %v", err)
	}
	if _, err := svc.SubmitItem(ctx, secondIntern.ID, itemID, &secondText, nil); err != nil {
		t.Fatalf("submit second intern result: %v", err)
	}

	history, err := svc.ListSubmissions(ctx, firstIntern.ID, itemID)
	if err != nil {
		t.Fatalf("list own history: %v", err)
	}
	if len(history) != 1 || history[0].ID != firstSubmission.ID {
		t.Fatalf("unexpected own history: %+v", history)
	}
	if _, err := svc.ListSubmissions(ctx, firstIntern.ID, uuid.New()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("foreign/missing progress: expected ErrNotFound, got %v", err)
	}

	queue, err := svc.ListPendingSubmissions(ctx, *firstIntern.BuddyID)
	if err != nil {
		t.Fatalf("list buddy queue: %v", err)
	}
	if len(queue) != 1 || queue[0].Submission.ID != firstSubmission.ID || queue[0].InternID != firstIntern.ID {
		t.Fatalf("buddy queue was not scoped: %+v", queue)
	}
	if _, err := svc.ListPendingSubmissions(ctx, firstIntern.ID); !errors.Is(err, ErrUserNotBuddy) {
		t.Fatalf("intern queue access: expected ErrUserNotBuddy, got %v", err)
	}
}

func stringPointer(value string) *string { return &value }

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
