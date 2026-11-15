package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestCleanupLoadGuardsAndTransactionalOwnership(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.HasPrefix(name, "ai_buddy_verify_") {
		t.Fatalf("unsafe DB %q", name)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_NAME", name)
	password, _ := u.User.Password()
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("DB_SSLMODE", "disable")
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	runID := "mixed-20261114-123456-" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	call := func(args ...string) error {
		flag.CommandLine = flag.NewFlagSet("cleanup-load-test", flag.ContinueOnError)
		os.Args = append([]string{"cleanup-load"}, args...)
		return run()
	}
	if err := call("-run-id", "wrong"); err == nil || !strings.Contains(err.Error(), "invalid run ID") {
		t.Fatalf("invalid run ID: %v", err)
	}
	if err := call("-run-id", runID, "-apply"); err == nil || !strings.Contains(err.Error(), "requires nonnegative") {
		t.Fatalf("apply guard: %v", err)
	}
	if err := call("-run-id", runID); err == nil || !strings.Contains(err.Error(), "refusing cleanup") {
		t.Fatalf("non-load database guard: %v", err)
	}
	profession := model.ProfessionBackend
	owner := &model.User{Email: "load.intern.001@example.invalid", PasswordHash: "test", Role: model.RoleIntern, Profession: &profession}
	other := &model.User{Email: "load-other-" + uuid.NewString() + "@example.invalid", PasswordHash: "test", Role: model.RoleIntern, Profession: &profession}
	users := repository.NewUserRepository(db)
	for _, user := range []*model.User{owner, other} {
		if err := users.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, owner.ID, other.ID)
	})
	title := fmt.Sprintf("load-vu-1-%s-1", runID)
	for _, user := range []*model.User{owner, other} {
		if _, err := db.ExecContext(ctx, `INSERT INTO notes(user_id,title,content) VALUES($1,$2,'Synthetic k6 note')`, user.ID, title); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO calendar_events(user_id,title,description,starts_at,ends_at) VALUES($1,$2,'Synthetic k6 event',$3,$4)`, user.ID, title, time.Now().UTC(), time.Now().UTC().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, marker string }{{"notes", "content = 'Synthetic k6 note'"}, {"calendar_events", "description = 'Synthetic k6 event'"}} {
		before, err := count(ctx, tx, tc.table, tc.marker, runID)
		if err != nil || before != 1 {
			t.Fatalf("%s owner filter count=%d err=%v", tc.table, before, err)
		}
		deleted, err := remove(ctx, tx, tc.table, tc.marker, runID)
		if err != nil || deleted != 1 {
			t.Fatalf("%s owner filter delete=%d err=%v", tc.table, deleted, err)
		}
		remaining, err := count(ctx, tx, tc.table, tc.marker, runID)
		if err != nil || remaining != 0 {
			t.Fatalf("%s transactional count=%d err=%v", tc.table, remaining, err)
		}
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, marker string }{{"notes", "content = 'Synthetic k6 note'"}, {"calendar_events", "description = 'Synthetic k6 event'"}} {
		check, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		preserved, err := count(ctx, check, tc.table, tc.marker, runID)
		_ = check.Rollback()
		if err != nil || preserved != 1 {
			t.Fatalf("%s rollback lost owner row: count=%d err=%v", tc.table, preserved, err)
		}
	}
}
