package main

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestMigrationHeadRejectsMissingAndReadsHighest(t *testing.T) {
	dir := t.TempDir()
	if _, err := migrationHead(dir); err == nil {
		t.Fatal("empty migration directory accepted")
	}
	for _, name := range []string{"readme.sql", "00001_initial.sql", "00009_next.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if head, err := migrationHead(dir); err != nil || head != 9 {
		t.Fatalf("head=%d err=%v", head, err)
	}
}

func TestSeedLoadRefusesNonLoadDatabaseAndHelpersAreIdempotent(t *testing.T) {
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
	var version, prior int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklists`).Scan(&prior); err != nil || prior != 0 {
		t.Fatalf("fixture requires empty test catalog: count=%d err=%v", prior, err)
	}
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("LOAD_USER_PASSWORD", "too-short")
	if err := run(); err == nil || !strings.Contains(err.Error(), "LOAD_USER_PASSWORD") {
		t.Fatalf("short password: %v", err)
	}
	t.Setenv("LOAD_USER_PASSWORD", "safe-test-password-only")
	if err := run(); err == nil || !strings.Contains(err.Error(), "refusing to seed database") {
		t.Fatalf("non-load DB guard: %v", err)
	}
	if err := ensureChecklists(ctx, db); err != nil {
		t.Fatal(err)
	}
	var common, professional uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type='common'`).Scan(&common); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type='professional' AND profession='backend'`).Scan(&professional); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM checklist_items WHERE checklist_id IN ($1,$2)`,
			`DELETE FROM checklist_stages WHERE checklist_id IN ($1,$2)`,
			`DELETE FROM checklists WHERE id IN ($1,$2)`,
		} {
			if _, err := db.ExecContext(context.Background(), query, common, professional); err != nil {
				t.Errorf("cleanup load fixture: %v", err)
			}
		}
	})
	if err := ensureChecklists(ctx, db); err != nil {
		t.Fatalf("repeat catalog creation: %v", err)
	}
	for _, id := range []uuid.UUID{common, professional} {
		var stages, items int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_stages WHERE checklist_id=$1`, id).Scan(&stages); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_items WHERE checklist_id=$1`, id).Scan(&items); err != nil {
			t.Fatal(err)
		}
		if stages != 1 || items != 1 {
			t.Fatalf("duplicate catalog fixtures: stages=%d items=%d", stages, items)
		}
	}
	email := "seed-helper-" + uuid.NewString() + "@example.test"
	users := repository.NewUserRepository(db)
	staff, err := ensureStaff(ctx, users, email, model.RoleBuddy, "hash")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, staff.ID) })
	again, err := ensureStaff(ctx, users, email, model.RoleBuddy, "hash")
	if err != nil || again.ID != staff.ID {
		t.Fatalf("staff idempotence: id=%s err=%v", again.ID, err)
	}
	if _, err := ensureStaff(ctx, users, email, model.RoleTeamlead, "hash"); err == nil {
		t.Fatal("accepted existing staff with wrong role")
	}
	if _, err := db.ExecContext(ctx, `UPDATE checklists SET name='Conflicting load common' WHERE id=$1`, common); err != nil {
		t.Fatal(err)
	}
	if err := ensureChecklists(ctx, db); err == nil {
		t.Fatal("accepted conflicting catalog")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklists WHERE id IN ($1,$2)`, common, professional).Scan(&count); err != nil || count != 2 {
		t.Fatalf("conflicting catalog changed rows: count=%d err=%v", count, err)
	}
	if _, err := users.GetByEmail(ctx, email); errors.Is(err, repository.ErrNotFound) {
		t.Fatal("staff disappeared after rejected catalog")
	}
}
