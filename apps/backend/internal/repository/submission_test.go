package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestSubmissionRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	var migrationVersion int64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied
	`).Scan(&migrationVersion); err != nil {
		t.Fatalf("get Goose version: %v", err)
	}
	if migrationVersion != 3 {
		t.Fatalf("expected Goose version 3, got %d", migrationVersion)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback()
	})

	buddyID := uuid.New()
	internID := uuid.New()
	checklistID := uuid.New()
	stageID := uuid.New()
	firstItemID := uuid.New()
	secondItemID := uuid.New()
	firstProgressID := uuid.New()
	secondProgressID := uuid.New()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, $2, 'hash', 'buddy')
	`, buddyID, "submission-buddy-"+uuid.NewString()+"@example.com")
	if err != nil {
		t.Fatalf("create buddy fixture: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (
			id, email, password_hash, role, profession, buddy_id
		)
		VALUES ($1, $2, 'hash', 'intern', 'backend', $3)
	`, internID, "submission-intern-"+uuid.NewString()+"@example.com", buddyID)
	if err != nil {
		t.Fatalf("create intern fixture: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO checklists (id, name, type)
		VALUES ($1, $2, 'common')
	`, checklistID, "Submission test "+checklistID.String())
	if err != nil {
		t.Fatalf("create checklist fixture: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO checklist_stages (
			id, checklist_id, title, week_start, week_end, position
		)
		VALUES ($1, $2, 'Submission stage', 1, 1, 1)
	`, stageID, checklistID)
	if err != nil {
		t.Fatalf("create stage fixture: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO checklist_items (
			id, checklist_id, stage_id, title, position, requires_review
		)
		VALUES
			($1, $2, $3, 'First submission item', 1, TRUE),
			($4, $2, $3, 'Second submission item', 2, TRUE)
	`, firstItemID, checklistID, stageID, secondItemID)
	if err != nil {
		t.Fatalf("create item fixtures: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_checklist_progress (
			id, user_id, checklist_id, checklist_item_id, status
		)
		VALUES
			($1, $2, $3, $4, 'active'),
			($5, $2, $3, $6, 'active')
	`,
		firstProgressID,
		internID,
		checklistID,
		firstItemID,
		secondProgressID,
		secondItemID,
	)
	if err != nil {
		t.Fatalf("create progress fixtures: %v", err)
	}

	repo := NewSubmissionRepository(tx)
	progressRepo := NewProgressRepository(tx)

	assertPGError := func(
		name string,
		code string,
		constraint string,
		op func() error,
	) {
		t.Helper()

		savepoint := "submission_" + name
		if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
			t.Fatalf("create savepoint %s: %v", name, err)
		}

		opErr := op()

		if _, err := tx.ExecContext(
			ctx,
			"ROLLBACK TO SAVEPOINT "+savepoint,
		); err != nil {
			t.Fatalf("rollback to savepoint %s: %v", name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"RELEASE SAVEPOINT "+savepoint,
		); err != nil {
			t.Fatalf("release savepoint %s: %v", name, err)
		}

		if opErr == nil {
			t.Fatalf("%s: expected PostgreSQL error", name)
		}

		var pgErr *pgconn.PgError
		if !errors.As(opErr, &pgErr) {
			t.Fatalf("%s: expected pgconn.PgError, got %v", name, opErr)
		}
		if pgErr.Code != code {
			t.Errorf("%s: expected PostgreSQL code %s, got %s", name, code, pgErr.Code)
		}
		if constraint != "" && pgErr.ConstraintName != constraint {
			t.Errorf(
				"%s: expected constraint %s, got %s",
				name,
				constraint,
				pgErr.ConstraintName,
			)
		}
	}

	firstText := "First immutable result"
	firstURL := "https://example.com/results/first"
	firstSubmittedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)

	first, err := repo.Create(
		ctx,
		firstProgressID,
		1,
		&firstText,
		&firstURL,
		firstSubmittedAt,
	)
	if err != nil {
		t.Fatalf("create first submission: %v", err)
	}
	if first.ID == uuid.Nil {
		t.Fatal("expected generated submission ID")
	}
	if first.Status != model.SubmissionPending {
		t.Errorf("expected pending status, got %q", first.Status)
	}
	if !first.SubmittedAt.Equal(firstSubmittedAt) {
		t.Errorf("expected submitted_at %v, got %v", firstSubmittedAt, first.SubmittedAt)
	}

	found, err := repo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("get submission by ID: %v", err)
	}
	if found.ResultText == nil || *found.ResultText != firstText {
		t.Errorf("unexpected result text: %v", found.ResultText)
	}
	if found.ResultURL == nil || *found.ResultURL != firstURL {
		t.Errorf("unexpected result URL: %v", found.ResultURL)
	}

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing submission, got %v", err)
	}

	pending, err := repo.GetPendingByProgressID(ctx, firstProgressID)
	if err != nil {
		t.Fatalf("get pending submission: %v", err)
	}
	if pending.ID != first.ID {
		t.Errorf("expected pending submission %s, got %s", first.ID, pending.ID)
	}

	secondProgressText := "Second progress result"
	secondProgressSubmittedAt := firstSubmittedAt.Add(-time.Minute)
	secondProgressSubmission, err := repo.Create(
		ctx,
		secondProgressID,
		1,
		&secondProgressText,
		nil,
		secondProgressSubmittedAt,
	)
	if err != nil {
		t.Fatalf("create second progress submission: %v", err)
	}

	pendingList, err := repo.ListPending(ctx)
	if err != nil {
		t.Fatalf("list pending submissions: %v", err)
	}
	firstIndex := -1
	secondIndex := -1
	for i := range pendingList {
		switch pendingList[i].ID {
		case first.ID:
			firstIndex = i
		case secondProgressSubmission.ID:
			secondIndex = i
		}
	}
	if firstIndex == -1 || secondIndex == -1 {
		t.Fatalf("pending queue does not contain test submissions: %v", pendingList)
	}
	if secondIndex >= firstIndex {
		t.Errorf("pending submissions are not ordered by submitted_at")
	}

	reviewedAt := time.Now().UTC().Truncate(time.Microsecond)
	reviewed, err := repo.Review(
		ctx,
		secondProgressSubmission.ID,
		model.SubmissionApproved,
		reviewedAt,
		buddyID,
		nil,
	)
	if err != nil {
		t.Fatalf("approve second progress submission: %v", err)
	}
	if !reviewed {
		t.Fatal("expected second progress submission to be approved")
	}

	reviewed, err = repo.Review(
		ctx,
		first.ID,
		model.SubmissionApproved,
		reviewedAt,
		buddyID,
		nil,
	)
	if err != nil {
		t.Fatalf("approve first submission: %v", err)
	}
	if !reviewed {
		t.Fatal("expected first submission to be approved")
	}

	reviewed, err = repo.Review(
		ctx,
		first.ID,
		model.SubmissionRejected,
		reviewedAt,
		buddyID,
		&firstText,
	)
	if err != nil {
		t.Fatalf("repeat review: %v", err)
	}
	if reviewed {
		t.Fatal("expected repeated review to report false")
	}

	approved, err := repo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("get approved submission: %v", err)
	}
	if approved.Status != model.SubmissionApproved {
		t.Errorf("expected approved status, got %q", approved.Status)
	}
	if approved.ResultText == nil || *approved.ResultText != firstText ||
		approved.ResultURL == nil || *approved.ResultURL != firstURL {
		t.Error("review changed immutable submission result")
	}

	if _, err := progressRepo.GetByIDForUpdate(
		ctx,
		firstProgressID,
	); err != nil {
		t.Fatalf("lock progress before attempt calculation: %v", err)
	}

	nextAttempt, err := repo.GetNextAttemptNumber(ctx, firstProgressID)
	if err != nil {
		t.Fatalf("get next attempt number: %v", err)
	}
	if nextAttempt != 2 {
		t.Errorf("expected next attempt number 2, got %d", nextAttempt)
	}

	secondText := "Second attempt"
	second, err := repo.Create(
		ctx,
		firstProgressID,
		nextAttempt,
		&secondText,
		nil,
		time.Now().UTC().Truncate(time.Microsecond),
	)
	if err != nil {
		t.Fatalf("create second attempt: %v", err)
	}

	if _, err := repo.GetByIDForUpdate(ctx, second.ID); err != nil {
		t.Fatalf("get submission for update: %v", err)
	}

	assertPGError(
		"reject_without_comment",
		"23514",
		"chk_checklist_item_submissions_review",
		func() error {
			_, err := repo.Review(
				ctx,
				second.ID,
				model.SubmissionRejected,
				reviewedAt,
				buddyID,
				nil,
			)
			return err
		},
	)

	rejectionComment := "Please add the missing evidence"
	reviewed, err = repo.Review(
		ctx,
		second.ID,
		model.SubmissionRejected,
		reviewedAt,
		buddyID,
		&rejectionComment,
	)
	if err != nil {
		t.Fatalf("reject second attempt: %v", err)
	}
	if !reviewed {
		t.Fatal("expected second attempt to be rejected")
	}

	rejected, err := repo.GetByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("get rejected submission: %v", err)
	}
	if rejected.Status != model.SubmissionRejected ||
		rejected.ReviewComment == nil ||
		*rejected.ReviewComment != rejectionComment {
		t.Errorf("unexpected rejected submission: %+v", rejected)
	}
	if rejected.ResultText == nil || *rejected.ResultText != secondText {
		t.Error("rejection changed immutable result text")
	}

	thirdURL := "https://example.com/results/third"
	third, err := repo.Create(
		ctx,
		firstProgressID,
		3,
		nil,
		&thirdURL,
		time.Now().UTC().Truncate(time.Microsecond),
	)
	if err != nil {
		t.Fatalf("create third attempt: %v", err)
	}

	assertPGError(
		"duplicate_pending",
		"23505",
		"uq_checklist_item_submissions_pending",
		func() error {
			text := "Another pending attempt"
			_, err := repo.Create(
				ctx,
				firstProgressID,
				4,
				&text,
				nil,
				time.Now().UTC(),
			)
			return err
		},
	)

	reviewed, err = repo.Review(
		ctx,
		third.ID,
		model.SubmissionApproved,
		reviewedAt,
		buddyID,
		nil,
	)
	if err != nil || !reviewed {
		t.Fatalf("approve third attempt: reviewed=%v, err=%v", reviewed, err)
	}

	assertPGError(
		"duplicate_attempt",
		"23505",
		"uq_checklist_item_submissions_attempt",
		func() error {
			text := "Duplicate attempt number"
			_, err := repo.Create(
				ctx,
				firstProgressID,
				3,
				&text,
				nil,
				time.Now().UTC(),
			)
			return err
		},
	)

	assertPGError(
		"empty_result",
		"23514",
		"chk_checklist_item_submissions_result",
		func() error {
			emptyText := "   "
			emptyURL := ""
			_, err := repo.Create(
				ctx,
				secondProgressID,
				2,
				&emptyText,
				&emptyURL,
				time.Now().UTC(),
			)
			return err
		},
	)

	assertPGError(
		"invalid_attempt_number",
		"23514",
		"chk_checklist_item_submissions_attempt_number",
		func() error {
			text := "Invalid attempt"
			_, err := repo.Create(
				ctx,
				secondProgressID,
				0,
				&text,
				nil,
				time.Now().UTC(),
			)
			return err
		},
	)

	assertPGError(
		"missing_progress",
		"23503",
		"fk_checklist_item_submissions_progress",
		func() error {
			text := "Missing progress"
			_, err := repo.Create(
				ctx,
				uuid.New(),
				1,
				&text,
				nil,
				time.Now().UTC(),
			)
			return err
		},
	)

	history, err := repo.ListByProgressID(ctx, firstProgressID)
	if err != nil {
		t.Fatalf("list submission history: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(history))
	}
	for i := range history {
		if history[i].AttemptNumber != i+1 {
			t.Errorf(
				"history[%d]: expected attempt %d, got %d",
				i,
				i+1,
				history[i].AttemptNumber,
			)
		}
	}

	emptyHistory, err := repo.ListByProgressID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("list empty submission history: %v", err)
	}
	if emptyHistory == nil || len(emptyHistory) != 0 {
		t.Errorf("expected non-nil empty history, got %v", emptyHistory)
	}

	if _, err := repo.GetPendingByProgressID(
		ctx,
		firstProgressID,
	); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected no pending submission after reviews, got %v", err)
	}

	rolledBackID := first.ID
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback repository transaction: %v", err)
	}

	dbRepo := NewSubmissionRepository(db)
	if _, err := dbRepo.GetByID(
		context.Background(),
		rolledBackID,
	); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected rolled back submission to be absent, got %v", err)
	}

	var persistedCount int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM checklist_item_submissions
		 WHERE id = $1`,
		rolledBackID,
	).Scan(&persistedCount); err != nil {
		t.Fatalf("count rolled back submission: %v", err)
	}
	if persistedCount != 0 {
		t.Errorf("expected rollback to remove submission, count=%d", persistedCount)
	}
}
