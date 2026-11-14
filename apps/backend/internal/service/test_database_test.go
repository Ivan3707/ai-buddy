package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func requireIsolatedTestDatabase(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()

	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatalf("read test database name: %v", err)
	}
	if name != "ai_buddy_test" && !strings.HasPrefix(name, "ai_buddy_verify_") {
		t.Fatalf("unsafe test database %q", name)
	}

	var version int
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied
	`).Scan(&version); err != nil {
		t.Fatalf("read Goose version: %v", err)
	}
	if version != 3 {
		t.Fatalf("test database must use Goose version 3, got %d", version)
	}
}
