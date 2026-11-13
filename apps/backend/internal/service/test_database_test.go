package service

import (
	"context"
	"database/sql"
	"testing"
)

func requireIsolatedTestDatabase(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()

	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatalf("read test database name: %v", err)
	}
	if name != "ai_buddy_test" {
		t.Fatalf("unsafe test database %q: expected ai_buddy_test", name)
	}

	var version int
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied
	`).Scan(&version); err != nil {
		t.Fatalf("read Goose version: %v", err)
	}
	if version != 9 {
		t.Fatalf("ai_buddy_test must use Goose version 9, got %d", version)
	}
}
