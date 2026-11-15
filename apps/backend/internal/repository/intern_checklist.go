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

// InternChecklistRepository stores per-intern edits of the checklists (migration 00007) and the demo
// "complete everything" operation. It never changes the shared catalog: every write goes to the
// intern's own user_checklist_progress rows. The caller owns the transaction.
type InternChecklistRepository struct {
	db DBTX
}

func NewInternChecklistRepository(db DBTX) *InternChecklistRepository {
	return &InternChecklistRepository{db: db}
}

// EditableProgress is one progress row of an intern together with what an edit needs to know about the item.
type EditableProgress struct {
	ID          uuid.UUID
	ChecklistID uuid.UUID
	Status      model.ChecklistStatus
	Hidden      bool
	Personal    bool
}

// TaskOverrides are the teamlead's replacements for catalog values; nil keeps the current value.
type TaskOverrides struct {
	Title            *string
	Description      *string
	ExpectedResult   *string
	EstimatedMinutes *int
	DueAt            *time.Time
}

// LockProgress locks the intern's progress row of an item (FOR UPDATE). ErrNotFound when the item is
// not in the intern's checklists.
func (r *InternChecklistRepository) LockProgress(ctx context.Context, userID, itemID uuid.UUID) (*EditableProgress, error) {
	var p EditableProgress
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT p.id, p.checklist_id, p.status, p.hidden_at IS NOT NULL, i.assigned_user_id IS NOT NULL
		FROM user_checklist_progress p JOIN checklist_items i ON i.id = p.checklist_item_id
		WHERE p.user_id = $1 AND p.checklist_item_id = $2
		FOR UPDATE OF p`, userID, itemID).Scan(&p.ID, &p.ChecklistID, &status, &p.Hidden, &p.Personal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock intern progress: %w", err)
	}
	p.Status = model.ChecklistStatus(status)
	return &p, nil
}

// Hide removes the item from the intern's checklist. History (submissions, comments, hints) stays.
func (r *InternChecklistRepository) Hide(ctx context.Context, progressID uuid.UUID) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE user_checklist_progress SET hidden_at = COALESCE(hidden_at, now()) WHERE id = $1`, progressID); err != nil {
		return fmt.Errorf("hide intern task: %w", err)
	}
	return nil
}

// SetOverrides stores the intern's own wording, estimate or deadline of the item.
func (r *InternChecklistRepository) SetOverrides(ctx context.Context, progressID uuid.UUID, o TaskOverrides) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_checklist_progress SET
		title_override = COALESCE($2, title_override),
		description_override = COALESCE($3, description_override),
		expected_result_override = COALESCE($4, expected_result_override),
		estimated_minutes_override = COALESCE($5, estimated_minutes_override),
		due_at_override = COALESCE($6, due_at_override)
		WHERE id = $1`, progressID, o.Title, o.Description, o.ExpectedResult, o.EstimatedMinutes, o.DueAt)
	if err != nil {
		return fmt.Errorf("update intern task: %w", err)
	}
	return nil
}

// ActivateFirstOpen keeps the sequential invariant after a shared item was removed: the first visible
// shared item that is not done must be active. It does nothing when that item is already active or
// when everything is done. Personal items are independent and are not considered. The opened item is
// available, not started (activated_at NULL): the intern takes it into work by hand.
func (r *InternChecklistRepository) ActivateFirstOpen(ctx context.Context, userID, checklistID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_checklist_progress SET status = 'active', activated_at = NULL
		WHERE status = 'unavailable' AND id = (
			SELECT p.id FROM user_checklist_progress p JOIN checklist_items i ON i.id = p.checklist_item_id
			WHERE p.user_id = $1 AND p.checklist_id = $2 AND p.hidden_at IS NULL AND i.assigned_user_id IS NULL
			  AND p.status IN ('unavailable', 'active')
			ORDER BY p.position, i.id LIMIT 1)`, userID, checklistID)
	if err != nil {
		return fmt.Errorf("activate next intern task: %w", err)
	}
	return nil
}

// DemoCompletion is what CompleteAll changed.
type DemoCompletion struct {
	ItemsCompleted      int64
	SubmissionsApproved int64
}

// DemoReviewComment marks submissions approved by the demo operation, so they are not mistaken for a real review.
const DemoReviewComment = "Демо: отмечено выполненным из админ-панели"

// CompleteAll marks every visible item of the intern's current checklists (common + current profession) done:
// items with review become verified, the rest completed; pending submissions are approved on behalf of the
// intern's buddy (or reviewerID when the intern has none). Already finished items keep completed_at, so a
// repeated call changes nothing (idempotent).
func (r *InternChecklistRepository) CompleteAll(ctx context.Context, userID, reviewerID uuid.UUID) (DemoCompletion, error) {
	var out DemoCompletion
	res, err := r.db.ExecContext(ctx, `UPDATE checklist_item_submissions s
		SET status = 'approved', reviewed_at = now(), reviewed_by = COALESCE(u.buddy_id, $2), review_comment = $3
		FROM user_checklist_progress p
		JOIN users u ON u.id = p.user_id
		JOIN checklists c ON c.id = p.checklist_id
		WHERE s.progress_id = p.id AND s.status = 'pending' AND p.user_id = $1 AND p.hidden_at IS NULL
		  AND (c.type = 'common' OR c.profession = u.profession)`, userID, reviewerID, DemoReviewComment)
	if err != nil {
		return out, fmt.Errorf("approve pending submissions: %w", err)
	}
	if out.SubmissionsApproved, err = res.RowsAffected(); err != nil {
		return out, err
	}
	res, err = r.db.ExecContext(ctx, `UPDATE user_checklist_progress p
		SET status = CASE WHEN i.requires_review THEN 'verified' ELSE 'completed' END,
		    completed_at = COALESCE(p.completed_at, now()), activated_at = NULL
		FROM checklist_items i, users u, checklists c
		WHERE i.id = p.checklist_item_id AND u.id = p.user_id AND c.id = p.checklist_id
		  AND p.user_id = $1 AND p.hidden_at IS NULL
		  AND (c.type = 'common' OR c.profession = u.profession)
		  AND NOT (p.status = 'verified' OR (p.status = 'completed' AND NOT i.requires_review))`, userID)
	if err != nil {
		return out, fmt.Errorf("complete intern checklists: %w", err)
	}
	if out.ItemsCompleted, err = res.RowsAffected(); err != nil {
		return out, err
	}
	return out, nil
}
