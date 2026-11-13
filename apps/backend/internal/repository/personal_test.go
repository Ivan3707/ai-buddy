package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"os"
	"testing"
	"time"
)

func TestPersonalRecordsOwnerIsolation(t *testing.T) {
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
	users := NewUserRepository(db)
	ids := make([]uuid.UUID, 2)
	for i := range ids {
		u := &model.User{Email: "owner-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
		if err = users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		ids[i] = u.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
	}
	repo := NewPersonalRepository(db)
	note := &Note{Title: "Private", Content: "text"}
	if err = repo.CreateNote(ctx, ids[0], note); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetNote(ctx, ids[1], note.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign note read: %v", err)
	}
	if err = repo.UpdateNote(ctx, ids[1], &Note{ID: note.ID, Title: "Changed"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign note update: %v", err)
	}
	if err = repo.DeleteNote(ctx, ids[1], note.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign note delete: %v", err)
	}
	start := time.Now().UTC()
	event := &CalendarEvent{Title: "Private", StartsAt: start, EndsAt: start.Add(time.Hour)}
	if err = repo.CreateEvent(ctx, ids[0], event); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetEvent(ctx, ids[1], event.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign event read: %v", err)
	}
	if err = repo.UpdateEvent(ctx, ids[1], event); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign event update: %v", err)
	}
	if err = repo.DeleteEvent(ctx, ids[1], event.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign event delete: %v", err)
	}
}
