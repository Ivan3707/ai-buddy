package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestProgressRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Создаём тестового пользователя.
	profession := model.ProfessionBackend

	testUser := &model.User{
		Email:        fmt.Sprintf("progress-test-%s@example.com", uuid.NewString()),
		PasswordHash: "test-hash",
		Role:         model.RoleIntern,
		Profession:   &profession,
	}

	userRepo := NewUserRepository(db)

	if err := userRepo.Create(ctx, testUser); err != nil {
		t.Fatalf("create test user: %v", err)
	}

	user := testUser

	// Создаём общий и профессиональный чек-листы.
	commonChecklistID := uuid.New()
	professionalChecklistID := uuid.New()

	_, err = db.ExecContext(ctx, `
		INSERT INTO checklists (id, name, type, profession)
		VALUES
			($1, 'Progress test common', 'common', NULL),
			($2, 'Progress test backend', 'professional', 'backend')
	`, commonChecklistID, professionalChecklistID)
	if err != nil {
		t.Fatalf("create test checklists: %v", err)
	}

	// Создаём по два задания в каждом чек-листе.
	commonFirstItemID := uuid.New()
	commonSecondItemID := uuid.New()
	professionalFirstItemID := uuid.New()
	professionalSecondItemID := uuid.New()

	_, err = db.ExecContext(ctx, `
		INSERT INTO checklist_items (
			id, checklist_id, title, description, position
		)
		VALUES
			($1, $2, 'Common 1', '', 1),
			($3, $2, 'Common 2', '', 2),
			($4, $5, 'Backend 1', '', 1),
			($6, $5, 'Backend 2', '', 2)
	`,
		commonFirstItemID,
		commonChecklistID,
		commonSecondItemID,
		professionalFirstItemID,
		professionalChecklistID,
		professionalSecondItemID,
	)
	if err != nil {
		t.Fatalf("create test checklist items: %v", err)
	}

	// Удаляем тестовые данные до закрытия соединения.
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx,
			`DELETE FROM user_checklist_progress WHERE user_id = $1`,
			user.ID,
		)
		_, _ = db.ExecContext(ctx,
			`DELETE FROM checklist_items WHERE checklist_id IN ($1, $2)`,
			commonChecklistID,
			professionalChecklistID,
		)
		_, _ = db.ExecContext(ctx,
			`DELETE FROM checklists WHERE id IN ($1, $2)`,
			commonChecklistID,
			professionalChecklistID,
		)
		_, _ = db.ExecContext(ctx,
			`DELETE FROM users WHERE id = $1`,
			user.ID,
		)
	})

	progressRepo := NewProgressRepository(db)

	// Создаём начальное состояние прогресса.
	initialProgress := []model.UserChecklistProgress{
		{
			UserID:          user.ID,
			ChecklistID:     commonChecklistID,
			ChecklistItemID: commonFirstItemID,
			Status:          model.StatusActive,
		},
		{
			UserID:          user.ID,
			ChecklistID:     commonChecklistID,
			ChecklistItemID: commonSecondItemID,
			Status:          model.StatusUnavailable,
		},
		{
			UserID:          user.ID,
			ChecklistID:     professionalChecklistID,
			ChecklistItemID: professionalFirstItemID,
			Status:          model.StatusActive,
		},
		{
			UserID:          user.ID,
			ChecklistID:     professionalChecklistID,
			ChecklistItemID: professionalSecondItemID,
			Status:          model.StatusUnavailable,
		},
	}

	if err := progressRepo.CreateMany(ctx, initialProgress); err != nil {
		t.Fatalf("create progress: %v", err)
	}

	// Проверяем получение всех записей и порядок.
	progress, err := progressRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get progress by user ID: %v", err)
	}

	if len(progress) != 4 {
		t.Fatalf("expected 4 progress records, got %d", len(progress))
	}

	expectedItemIDs := []uuid.UUID{
		commonFirstItemID,
		commonSecondItemID,
		professionalFirstItemID,
		professionalSecondItemID,
	}

	for i, item := range progress {
		if item.ChecklistItemID != expectedItemIDs[i] {
			t.Errorf(
				"progress[%d]: expected item %s, got %s",
				i,
				expectedItemIDs[i],
				item.ChecklistItemID,
			)
		}

		if item.CompletedAt != nil {
			t.Errorf("progress[%d]: expected nil completed_at", i)
		}
	}

	// Проверяем получение конкретной записи внутри транзакции.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	txRepo := NewProgressRepository(tx)

	item, err := txRepo.GetByUserAndItem(
		ctx,
		user.ID,
		commonFirstItemID,
	)
	if err != nil {
		t.Fatalf("get progress by user and item: %v", err)
	}

	if item.Status != model.StatusActive {
		t.Errorf("expected active status, got %s", item.Status)
	}

	_, err = txRepo.GetByUserAndItem(ctx, user.ID, uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	// Проверяем перевод задания в completed.
	completedAt := time.Now().UTC().Truncate(time.Microsecond)

	err = progressRepo.UpdateStatus(
		ctx,
		user.ID,
		commonFirstItemID,
		model.StatusCompleted,
		&completedAt,
	)
	if err != nil {
		t.Fatalf("update progress status: %v", err)
	}

	progress, err = progressRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get updated progress: %v", err)
	}

	var completedItem *model.UserChecklistProgress

	for i := range progress {
		if progress[i].ChecklistItemID == commonFirstItemID {
			completedItem = &progress[i]
			break
		}
	}

	if completedItem == nil {
		t.Fatal("completed item not found")
	}

	if completedItem.Status != model.StatusCompleted {
		t.Errorf(
			"expected completed status, got %s",
			completedItem.Status,
		)
	}

	if completedItem.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}

	if !completedItem.CompletedAt.Equal(completedAt) {
		t.Errorf(
			"expected completed_at %v, got %v",
			completedAt,
			*completedItem.CompletedAt,
		)
	}

	// Проверяем обновление несуществующей записи.
	err = progressRepo.UpdateStatus(
		ctx,
		user.ID,
		uuid.New(),
		model.StatusCompleted,
		&completedAt,
	)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
