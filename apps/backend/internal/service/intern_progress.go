package service

import (
	"context"
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
		repo := repository.NewProgressRepository(tx)
		checklist, err := repo.ChecklistForTrack(ctx, typ, profession)
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("missing %s checklist for profession %s", typ, profession)
		}
		if err != nil {
			return err
		}
		if err := repo.InitializeChecklist(ctx, userID, checklist); err != nil {
			return fmt.Errorf("insert %s progress: %w", typ, err)
		}
	}
	return nil
}
