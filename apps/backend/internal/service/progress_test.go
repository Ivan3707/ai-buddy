package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

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
