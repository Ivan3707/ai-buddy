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

func TestSessionRotationLockTimeoutPreservesToken(t *testing.T) {
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
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	requireIsolatedTestDatabase(t, db, ctx)
	user := &model.User{Email: "session-failure-" + uuid.NewString() + "@example.test", PasswordHash: "hash", Role: model.RoleBuddy}
	if err := NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID) })
	repo := NewSessionRepository(db)
	id, err := repo.Create(ctx, user.ID, "original", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	locker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Rollback()
	var lockedID uuid.UUID
	if err := locker.QueryRowContext(ctx, `SELECT id FROM auth_sessions WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}
	deadlineCtx, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	defer stop()
	if _, err := repo.Rotate(deadlineCtx, id, "original", "never-committed", time.Now().Add(time.Hour)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("locked rotation should time out, got %v", err)
	}
	if err := locker.Rollback(); err != nil {
		t.Fatal(err)
	}
	owner, err := repo.Rotate(ctx, id, "original", "now-committed", time.Now().Add(time.Hour))
	if err != nil || owner != user.ID {
		t.Fatalf("failed rotation changed original token: owner=%s err=%v", owner, err)
	}
	if err := repo.Revoke(ctx, id, user.ID, "original"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token revoked rotated session: %v", err)
	}
	active, err := repo.IsActive(ctx, id, user.ID)
	if err != nil || !active {
		t.Fatalf("session after wrong revoke: active=%t err=%v", active, err)
	}
	cancelled, stopNow := context.WithCancel(ctx)
	stopNow()
	if err := repo.Revoke(cancelled, id, user.ID, "now-committed"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled revoke: %v", err)
	}
	active, err = repo.IsActive(ctx, id, user.ID)
	if err != nil || !active {
		t.Fatalf("cancelled revoke changed session: active=%t err=%v", active, err)
	}
}
