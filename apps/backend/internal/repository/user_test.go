package repository

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
)

func TestUserRepository(t *testing.T) {
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

	repo := NewUserRepository(db)

	email := "test-" + uuid.NewString() + "@example.com"
	profession := model.ProfessionBackend

	user := &model.User{
		Email:        email,
		PasswordHash: "test-hash",
		Role:         model.RoleIntern,
		Profession:   &profession,
	}

	// Удаляем тестового пользователя даже при неудачном тесте.
	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE email = $1",
			email,
		)
		if err != nil {
			t.Errorf("cleanup test user: %v", err)
		}
	})

	// Create
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if user.ID == uuid.Nil {
		t.Fatal("expected generated user ID")
	}

	if user.CreatedAt.IsZero() {
		t.Fatal("expected generated created_at")
	}

	// GetByID
	foundByID, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user by ID: %v", err)
	}

	if foundByID.Email != email {
		t.Errorf("expected email %q, got %q", email, foundByID.Email)
	}

	if foundByID.Profession == nil ||
		*foundByID.Profession != profession {
		t.Errorf("unexpected profession: %v", foundByID.Profession)
	}

	// GetByEmail: проверяем регистронезависимый поиск.
	foundByEmail, err := repo.GetByEmail(ctx, "TEST-"+email[len("test-"):])
	if err != nil {
		t.Fatalf("get user by email: %v", err)
	}

	if foundByEmail.ID != user.ID {
		t.Errorf("expected ID %s, got %s", user.ID, foundByEmail.ID)
	}

	// Not found
	_, err = repo.GetByID(ctx, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Duplicate email
	duplicate := &model.User{
		Email:        email,
		PasswordHash: "another-hash",
		Role:         model.RoleIntern,
		Profession:   &profession,
	}

	err = repo.Create(ctx, duplicate)
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Errorf("expected ErrEmailAlreadyExists, got %v", err)
	}
}
