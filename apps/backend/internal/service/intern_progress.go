package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// InitializeInternProgress is the single implementation of the onboarding
// progress invariant: an intern gets every shared item of the common checklist
// and of the professional checklist for their profession; the first item of
// each checklist becomes active but not started (activated_at NULL: the intern takes it into work by
// hand via StartItem), the rest are unavailable. Items that already
// have progress are kept untouched, so the call is idempotent.
//
// Accounts themselves are created only by HRSynchronizer (Mock HR). The helper
// is exported for synthetic fixtures (tests, cmd/seed-load) that insert users
// directly; it must run inside the transaction that created the user.
func InitializeInternProgress(ctx context.Context, tx repository.DBTX, userID uuid.UUID, profession model.Profession) error {
	switch profession {
	case model.ProfessionBackend, model.ProfessionFrontend, model.ProfessionML:
	default:
		return ErrInvalidProfession
	}
	for _, typ := range []string{"common", "professional"} {
		var checklist uuid.UUID
		err := tx.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type=$1 AND ($1='common' OR profession=$2)`, typ, string(profession)).Scan(&checklist)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("missing %s checklist for profession %s", typ, profession)
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at)
    SELECT $1,$2,i.id,CASE WHEN i.position=(SELECT MIN(position) FROM checklist_items WHERE checklist_id=$2 AND (assigned_user_id IS NULL OR assigned_user_id=$1))
      AND NOT EXISTS(SELECT 1 FROM user_checklist_progress p0 WHERE p0.user_id=$1 AND p0.checklist_id=$2) THEN 'active' ELSE 'unavailable' END,
    i.position,NULL
    FROM checklist_items i WHERE i.checklist_id=$2 AND (i.assigned_user_id IS NULL OR i.assigned_user_id=$1)
    AND NOT EXISTS(SELECT 1 FROM user_checklist_progress p WHERE p.user_id=$1 AND p.checklist_item_id=i.id)`, userID, checklist)
		if err != nil {
			return fmt.Errorf("insert %s progress: %w", typ, err)
		}
	}
	return nil
}
