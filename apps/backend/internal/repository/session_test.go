package repository

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"os"
	"testing"
	"time"
)

func TestSessionRotationReplayRevokes(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	requireIsolatedTestDatabase(t, db, ctx)
	user := &model.User{Email: "session-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
	if err = NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID) })
	repo := NewSessionRepository(db)
	id, err := repo.Create(ctx, user.ID, "original", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := repo.Rotate(ctx, id, "original", "new", time.Now().Add(time.Hour))
	if err != nil || owner != user.ID {
		t.Fatalf("rotate: %s %v", owner, err)
	}
	if err = repo.Revoke(ctx, id, user.ID, "wrong"); err != ErrNotFound {
		t.Fatalf("invalid logout secret should be rejected: %v", err)
	}
	active, err := repo.IsActive(ctx, id, user.ID)
	if err != nil || !active {
		t.Fatalf("invalid logout secret revoked session: active=%v err=%v", active, err)
	}
	if _, err = repo.Rotate(ctx, id, "original", "third", time.Now().Add(time.Hour)); err != ErrNotFound {
		t.Fatalf("replay should be rejected: %v", err)
	}
	active, err = repo.IsActive(ctx, id, user.ID)
	if err != nil || active {
		t.Fatalf("replay should revoke session: active=%v err=%v", active, err)
	}
}
