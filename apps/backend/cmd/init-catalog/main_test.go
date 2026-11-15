package main

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCatalogScriptBodyRejectsMissingMarkers(t *testing.T) {
	for _, body := range []string{"SELECT 1", "\nBEGIN;\nSELECT 1", "\nCOMMIT;"} {
		if _, err := scriptBody(body); err == nil {
			t.Fatalf("accepted incomplete transaction markers: %q", body)
		}
	}
	for _, name := range catalogScripts {
		data, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		body, err := scriptBody(string(data))
		if err != nil || !strings.Contains(body, "INSERT") {
			t.Fatalf("%s: missing executable catalog body: %v", name, err)
		}
	}
}

func TestInitCatalogCLIIdempotentOnIsolatedDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_CATALOG_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_CATALOG_DATABASE_URL is not set")
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
	defer db.Close()
	ctx := context.Background()
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_SSLMODE", "disable")
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	for attempt := 1; attempt <= 2; attempt++ {
		if err := run(); err != nil {
			t.Fatalf("catalog run %d: %v", attempt, err)
		}
		var checklists, stages, items int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklists`).Scan(&checklists); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_stages`).Scan(&stages); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_items`).Scan(&items); err != nil {
			t.Fatal(err)
		}
		if checklists != 4 || stages != 12 || items != 36 {
			t.Fatalf("run %d catalog counts: checklists=%d stages=%d items=%d", attempt, checklists, stages, items)
		}
	}
}
