package service

import (
	"errors"
	"testing"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestProgressConcurrentCompletionsAndChecklistIsolation(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 3, 2)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	svc := NewProgressService(db)
	type operation struct {
		item         int
		professional bool
	}
	operations := []operation{{0, false}, {0, false}, {1, false}, {0, true}}
	results := make(chan struct {
		operation operation
		err       error
	}, len(operations))
	start := make(chan struct{})
	for _, op := range operations {
		go func(op operation) {
			<-start
			id := fixture.commonItems[op.item].ID
			if op.professional {
				id = fixture.profItems[op.item].ID
			}
			results <- struct {
				operation operation
				err       error
			}{op, svc.CompleteItem(ctx, user.ID, id)}
		}(op)
	}
	close(start)
	for range operations {
		got := <-results
		if got.operation.item == 1 && !got.operation.professional {
			if got.err != nil && !errors.Is(got.err, ErrItemNotActive) {
				t.Fatalf("adjacent common item: %v", got.err)
			}
		} else if got.err != nil {
			t.Fatalf("concurrent completion: %+v", got)
		}
	}
	firstCommon := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[0].ID)
	secondCommon := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[1].ID)
	thirdCommon := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[2].ID)
	firstProfessional := getProgressForTest(t, db, ctx, user.ID, fixture.profItems[0].ID)
	secondProfessional := getProgressForTest(t, db, ctx, user.ID, fixture.profItems[1].ID)
	if firstCommon.Status != model.StatusCompleted || firstProfessional.Status != model.StatusCompleted || secondProfessional.Status != model.StatusActive {
		t.Fatalf("checklist isolation: common=%s professional=%s next=%s", firstCommon.Status, firstProfessional.Status, secondProfessional.Status)
	}
	switch secondCommon.Status {
	case model.StatusActive:
		if thirdCommon.Status != model.StatusUnavailable {
			t.Fatalf("ordinary sequence skipped item 2: %s", thirdCommon.Status)
		}
	case model.StatusCompleted:
		if thirdCommon.Status != model.StatusActive {
			t.Fatalf("ordinary sequence did not open item 3: %s", thirdCommon.Status)
		}
	default:
		t.Fatalf("invalid adjacent state: item2=%s item3=%s", secondCommon.Status, thirdCommon.Status)
	}
}

func TestProgressConcurrentApproveAndReject(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 1)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	firstID := fixture.commonItems[0].ID
	setRequiresReview(t, db, ctx, firstID)
	svc := NewProgressService(db)
	text, comment := "result", "needs work"
	submission, err := svc.SubmitItem(ctx, user.ID, firstID, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	go func() { <-start; results <- svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, true, nil) }()
	go func() { <-start; results <- svc.ReviewSubmission(ctx, *user.BuddyID, submission.ID, false, &comment) }()
	close(start)
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrSubmissionAlreadyReviewed):
			conflict++
		default:
			t.Fatalf("unexpected review error: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("review outcomes: success=%d conflict=%d", success, conflict)
	}
	progress := getProgressForTest(t, db, ctx, user.ID, firstID)
	if progress.Status != model.StatusVerified && progress.Status != model.StatusActive {
		t.Fatalf("invalid final review status: %s", progress.Status)
	}
	if got := getProgressForTest(t, db, ctx, user.ID, fixture.commonItems[1].ID).Status; got != model.StatusActive {
		t.Fatalf("review changed next ordinary task: %s", got)
	}
	history, err := repository.NewSubmissionRepository(db).ListByProgressID(ctx, progress.ID)
	if err != nil || len(history) != 1 || history[0].Status == model.SubmissionPending {
		t.Fatalf("review history: %+v err=%v", history, err)
	}
	if progress.Status == model.StatusVerified && history[0].Status != model.SubmissionApproved {
		t.Fatalf("verified progress with %s submission", history[0].Status)
	}
	if progress.Status == model.StatusActive && history[0].Status != model.SubmissionRejected {
		t.Fatalf("reopened progress with %s submission", history[0].Status)
	}
}
