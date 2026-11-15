package service

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestHintEnqueueAndClaimOnlyOnceIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	requireIsolatedTestDatabase(t, db, ctx)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	buddy := &model.User{Email: "buddy-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
	if err = repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, buddy.ID) })
	intern, err := createTestIntern(ctx, db, "hint-"+uuid.NewString()+"@example.com", "hash", model.ProfessionBackend, buddy.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, intern.ID) })
	itemID := fixture.profItems[0].ID
	if _, err = db.ExecContext(ctx, `UPDATE checklist_items SET estimated_minutes=1 WHERE id=$1`, itemID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE user_checklist_progress SET activated_at=now()-interval '2 minutes' WHERE user_id=$1 AND checklist_item_id=$2`, intern.ID, itemID); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewHintRepository(db)
	for i := 0; i < 2; i++ {
		if err = repo.EnqueueDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	due, err := repo.Claim(ctx)
	if err != nil || due == nil {
		t.Fatalf("first claim: %+v %v", due, err)
	}
	again, err := repo.Claim(ctx)
	if err != nil || again != nil {
		t.Fatalf("duplicate claim: %+v %v", again, err)
	}
	if err = repo.Resolve(ctx, due.ID, "Test hint", true); err != nil {
		t.Fatal(err)
	}
	if err = repo.EnqueueDue(ctx); err != nil {
		t.Fatal(err)
	}
	hints, err := repo.List(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || hints[0].Text != "Test hint" {
		t.Fatalf("unexpected hints: %+v", hints)
	}
	if _, err = repo.GetByItem(ctx, buddy.ID, itemID); err != repository.ErrNotFound {
		t.Fatalf("foreign hint read: %v", err)
	}
	if err = repo.MarkRead(ctx, buddy.ID, hints[0].ID); err != repository.ErrNotFound {
		t.Fatalf("foreign hint mark read: %v", err)
	}
	if err = repo.MarkRead(ctx, intern.ID, hints[0].ID); err != nil {
		t.Fatal(err)
	}
	read, err := repo.GetByItem(ctx, intern.ID, itemID)
	if err != nil || read.ReadAt == nil {
		t.Fatalf("hint read_at: %+v %v", read, err)
	}
}
