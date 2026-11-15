package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestInitializeInternProgress_CommonAndProfessional(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	requireIsolatedTestDatabase(t, db, ctx)

	// Подготавливаем тестовые чек-листы.
	profession := model.ProfessionBackend

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		profession,
		3,
		2,
	)

	commonID := fixture.commonID
	professionalID := fixture.professionalID

	buddy := &model.User{
		Email:        "buddy-" + uuid.NewString() + "@example.com",
		PasswordHash: "test-password-hash",
		Role:         model.RoleBuddy,
	}

	if err := repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatalf("create buddy: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			buddy.ID,
		)
		if err != nil {
			t.Errorf("cleanup buddy: %v", err)
		}
	})

	// Создаём стажёра через общий инициализатор прогресса (как HR sync).
	email := "test-" + uuid.NewString() + "@example.com"

	user, err := createTestIntern(
		ctx,
		db,
		email,
		"test-password-hash",
		profession,
		buddy.ID,
	)
	if err != nil {
		t.Fatalf("create intern: %v", err)
	}

	// Проверяем пользователя.
	if user.ID == uuid.Nil {
		t.Fatal("expected generated user ID")
	}
	if user.Role != model.RoleIntern {
		t.Errorf("expected role %q, got %q", model.RoleIntern, user.Role)
	}
	if user.Profession == nil || *user.Profession != profession {
		t.Errorf("unexpected profession: %v", user.Profession)
	}

	// Проверяем прогресс.
	progressRepo := repository.NewProgressRepository(db)

	progress, err := progressRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user progress: %v", err)
	}

	if len(progress) != 5 {
		t.Fatalf("expected 5 progress records, got %d", len(progress))
	}

	activeByChecklist := make(map[uuid.UUID]int)

	for _, item := range progress {
		switch item.Status {
		case model.StatusActive:
			activeByChecklist[item.ChecklistID]++

		case model.StatusUnavailable:
			// Ожидаемый статус для последующих заданий.

		default:
			t.Errorf("unexpected status %q", item.Status)
		}
	}

	if len(activeByChecklist) != 2 {
		t.Fatalf("expected 2 checklists with active items, got %d", len(activeByChecklist))
	}

	if activeByChecklist[commonID] != 1 {
		t.Errorf("expected 1 active common item, got %d", activeByChecklist[commonID])
	}

	if activeByChecklist[professionalID] != 1 {
		t.Errorf("expected 1 active professional item, got %d", activeByChecklist[professionalID])
	}

	// Повторная инициализация идемпотентна: прогресс не дублируется.
	if err := withTransaction(ctx, db, func(tx *sql.Tx) error {
		return InitializeInternProgress(ctx, tx, user.ID, profession)
	}); err != nil {
		t.Fatalf("repeat progress initialization: %v", err)
	}
	again, err := progressRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user progress again: %v", err)
	}
	if len(again) != len(progress) {
		t.Fatalf("repeat initialization changed progress: %d -> %d", len(progress), len(again))
	}

	// Удаляем пользователя отдельно: тест проверяет и его создание.
	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID); err != nil {
		t.Errorf("cleanup test user: %v", err)
	}

}

func TestInitializeInternProgressRejectsUnknownProfession(t *testing.T) {
	// Проверка выполняется до обращения к БД, поэтому nil-соединение безопасно.
	err := InitializeInternProgress(context.Background(), nil, uuid.New(), model.Profession("designer"))
	if !errors.Is(err, ErrInvalidProfession) {
		t.Fatalf("expected ErrInvalidProfession, got %v", err)
	}
}

func TestInitializeInternProgress_RollbackWithoutProfessionalChecklist(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	requireIsolatedTestDatabase(t, db, ctx)

	// Создаём только общий чек-лист.
	commonID := uuid.New()

	_, err = db.ExecContext(ctx, `
		INSERT INTO checklists (id, name, type)
		VALUES ($1, $2, 'common')
	`, commonID, "Rollback test "+commonID.String())
	if err != nil {
		t.Fatalf("create common checklist: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM checklists WHERE id = $1",
			commonID,
		)
		if err != nil {
			t.Errorf("cleanup test checklist: %v", err)
		}
	})

	userRepo := repository.NewUserRepository(db)

	email := "rollback-" + uuid.NewString() + "@example.com"
	profession := model.ProfessionBackend

	buddy := &model.User{
		Email:        "buddy-" + uuid.NewString() + "@example.com",
		PasswordHash: "test-password-hash",
		Role:         model.RoleBuddy,
	}

	if err := repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatalf("create buddy: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			buddy.ID,
		)
		if err != nil {
			t.Errorf("cleanup buddy: %v", err)
		}
	})

	// Профессионального чек-листа нет.
	_, err = createTestIntern(
		ctx,
		db,
		email,
		"test-password-hash",
		profession,
		buddy.ID,
	)

	if err == nil {
		t.Fatal("expected error because professional checklist is missing")
	}

	// Проверяем, что пользователь не сохранился.
	_, err = userRepo.GetByEmail(ctx, email)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected user not to exist, got error: %v", err)
	}
}
