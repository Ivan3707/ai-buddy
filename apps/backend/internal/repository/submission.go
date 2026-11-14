package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type SubmissionRepository struct {
	db DBTX
}

type BuddyPendingSubmission struct {
	Submission      model.ChecklistItemSubmission
	InternID        uuid.UUID
	ItemID          uuid.UUID
	ItemTitle       string
	InternFirstName *string
	InternLastName  *string
}

func NewSubmissionRepository(db DBTX) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

func (r *SubmissionRepository) Create(
	ctx context.Context,
	progressID uuid.UUID,
	attemptNumber int,
	resultText *string,
	resultURL *string,
	submittedAt time.Time,
) (*model.ChecklistItemSubmission, error) {
	query := `
		INSERT INTO checklist_item_submissions (
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at
		)
		VALUES ($1, $2, $3, $4, 'pending', $5)
		RETURNING
			id,
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at,
			reviewed_at,
			reviewed_by,
			review_comment
	`

	submission, err := scanSubmission(r.db.QueryRowContext(
		ctx,
		query,
		progressID,
		attemptNumber,
		resultText,
		resultURL,
		submittedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("create checklist item submission: %w", err)
	}

	return submission, nil
}

func (r *SubmissionRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.ChecklistItemSubmission, error) {
	return r.getByID(ctx, id, false)
}

// GetByIDForUpdate must be called through a repository backed by an external
// transaction. The repository does not start or commit transactions itself.
func (r *SubmissionRepository) GetByIDForUpdate(
	ctx context.Context,
	id uuid.UUID,
) (*model.ChecklistItemSubmission, error) {
	return r.getByID(ctx, id, true)
}

func (r *SubmissionRepository) getByID(
	ctx context.Context,
	id uuid.UUID,
	forUpdate bool,
) (*model.ChecklistItemSubmission, error) {
	query := `
		SELECT
			id,
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at,
			reviewed_at,
			reviewed_by,
			review_comment
		FROM checklist_item_submissions
		WHERE id = $1
	`
	if forUpdate {
		query += " FOR UPDATE"
	}

	submission, err := scanSubmission(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get checklist item submission: %w", err)
	}

	return submission, nil
}

func (r *SubmissionRepository) GetPendingByProgressID(
	ctx context.Context,
	progressID uuid.UUID,
) (*model.ChecklistItemSubmission, error) {
	query := `
		SELECT
			id,
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at,
			reviewed_at,
			reviewed_by,
			review_comment
		FROM checklist_item_submissions
		WHERE progress_id = $1 AND status = 'pending'
	`

	submission, err := scanSubmission(
		r.db.QueryRowContext(ctx, query, progressID),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get pending checklist item submission: %w", err)
	}

	return submission, nil
}

// GetNextAttemptNumber is safe only after the caller locks the corresponding
// progress row with SELECT ... FOR UPDATE in the same external transaction.
// The database UNIQUE constraint remains the final protection against races.
func (r *SubmissionRepository) GetNextAttemptNumber(
	ctx context.Context,
	progressID uuid.UUID,
) (int, error) {
	query := `
		SELECT COALESCE(MAX(attempt_number), 0) + 1
		FROM checklist_item_submissions
		WHERE progress_id = $1
	`

	var attemptNumber int
	if err := r.db.QueryRowContext(ctx, query, progressID).Scan(
		&attemptNumber,
	); err != nil {
		return 0, fmt.Errorf("get next submission attempt number: %w", err)
	}

	return attemptNumber, nil
}

func (r *SubmissionRepository) ListByProgressID(
	ctx context.Context,
	progressID uuid.UUID,
) ([]model.ChecklistItemSubmission, error) {
	query := `
		SELECT
			id,
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at,
			reviewed_at,
			reviewed_by,
			review_comment
		FROM checklist_item_submissions
		WHERE progress_id = $1
		ORDER BY attempt_number ASC
	`

	return r.list(ctx, query, progressID)
}

func (r *SubmissionRepository) ListPending(
	ctx context.Context,
) ([]model.ChecklistItemSubmission, error) {
	query := `
		SELECT
			id,
			progress_id,
			attempt_number,
			result_text,
			result_url,
			status,
			submitted_at,
			reviewed_at,
			reviewed_by,
			review_comment
		FROM checklist_item_submissions
		WHERE status = 'pending'
		ORDER BY submitted_at ASC, id ASC
	`

	return r.list(ctx, query)
}

func (r *SubmissionRepository) ListPendingByBuddyID(
	ctx context.Context,
	buddyID uuid.UUID,
) ([]BuddyPendingSubmission, error) {
	query := `
		SELECT
			s.id,
			s.progress_id,
			s.attempt_number,
			s.result_text,
			s.result_url,
			s.status,
			s.submitted_at,
			s.reviewed_at,
			s.reviewed_by,
			s.review_comment,
			p.user_id,
			p.checklist_item_id,
			ci.title, u.first_name, u.last_name
		FROM checklist_item_submissions AS s
		JOIN user_checklist_progress AS p ON p.id = s.progress_id
		JOIN users AS u ON u.id = p.user_id
		JOIN checklist_items AS ci ON ci.id = p.checklist_item_id
		WHERE s.status = 'pending' AND u.buddy_id = $1
		ORDER BY s.submitted_at ASC, s.id ASC
	`

	rows, err := r.db.QueryContext(ctx, query, buddyID)
	if err != nil {
		return nil, fmt.Errorf("list pending submissions by buddy ID: %w", err)
	}
	defer rows.Close()

	result := make([]BuddyPendingSubmission, 0)
	for rows.Next() {
		var item BuddyPendingSubmission
		var resultText sql.NullString
		var resultURL sql.NullString
		var reviewedAt sql.NullTime
		var reviewedBy uuid.NullUUID
		var reviewComment sql.NullString
		var firstName, lastName sql.NullString
		if err := rows.Scan(
			&item.Submission.ID,
			&item.Submission.ProgressID,
			&item.Submission.AttemptNumber,
			&resultText,
			&resultURL,
			&item.Submission.Status,
			&item.Submission.SubmittedAt,
			&reviewedAt,
			&reviewedBy,
			&reviewComment,
			&item.InternID,
			&item.ItemID,
			&item.ItemTitle,
			&firstName, &lastName,
		); err != nil {
			return nil, fmt.Errorf("scan pending buddy submission: %w", err)
		}
		if resultText.Valid {
			item.Submission.ResultText = &resultText.String
		}
		if firstName.Valid {
			item.InternFirstName = &firstName.String
		}
		if lastName.Valid {
			item.InternLastName = &lastName.String
		}
		if resultURL.Valid {
			item.Submission.ResultURL = &resultURL.String
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending buddy submissions: %w", err)
	}

	return result, nil
}

func (r *SubmissionRepository) list(
	ctx context.Context,
	query string,
	args ...any,
) ([]model.ChecklistItemSubmission, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list checklist item submissions: %w", err)
	}
	defer rows.Close()

	submissions := make([]model.ChecklistItemSubmission, 0)
	for rows.Next() {
		submission, err := scanSubmission(rows)
		if err != nil {
			return nil, fmt.Errorf("scan checklist item submission: %w", err)
		}
		submissions = append(submissions, *submission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checklist item submissions: %w", err)
	}

	return submissions, nil
}

// Review reports false when the submission is missing or has already left
// pending state. The caller can distinguish that outcome from a successful
// atomic decision without the repository making authorization decisions.
func (r *SubmissionRepository) Review(
	ctx context.Context,
	id uuid.UUID,
	status model.SubmissionStatus,
	reviewedAt time.Time,
	reviewedBy uuid.UUID,
	reviewComment *string,
) (bool, error) {
	query := `
		UPDATE checklist_item_submissions
		SET
			status = $1,
			reviewed_at = $2,
			reviewed_by = $3,
			review_comment = $4
		WHERE id = $5 AND status = 'pending'
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		string(status),
		reviewedAt,
		reviewedBy,
		reviewComment,
		id,
	)
	if err != nil {
		return false, fmt.Errorf("review checklist item submission: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get reviewed submission rows: %w", err)
	}

	return affected == 1, nil
}

type submissionScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(
	scanner submissionScanner,
) (*model.ChecklistItemSubmission, error) {
	var submission model.ChecklistItemSubmission
	var resultText sql.NullString
	var resultURL sql.NullString
	var reviewedAt sql.NullTime
	var reviewedBy uuid.NullUUID
	var reviewComment sql.NullString

	if err := scanner.Scan(
		&submission.ID,
		&submission.ProgressID,
		&submission.AttemptNumber,
		&resultText,
		&resultURL,
		&submission.Status,
		&submission.SubmittedAt,
		&reviewedAt,
		&reviewedBy,
		&reviewComment,
	); err != nil {
		return nil, err
	}

	if resultText.Valid {
		submission.ResultText = &resultText.String
	}
	if resultURL.Valid {
		submission.ResultURL = &resultURL.String
	}
	if reviewedAt.Valid {
		submission.ReviewedAt = &reviewedAt.Time
	}
	if reviewedBy.Valid {
		submission.ReviewedBy = &reviewedBy.UUID
	}
	if reviewComment.Valid {
		submission.ReviewComment = &reviewComment.String
	}

	return &submission, nil
}
