package app

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRunReportsStartupConfigurationFailures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	if err := Run(); err == nil || !strings.Contains(err.Error(), "read corporate mock") {
		t.Fatalf("missing HR snapshot: %v", err)
	}
	path := filepath.Join(dir, "corporate_employees.json")
	if err := os.WriteFile(path, []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run(); err == nil || !strings.Contains(err.Error(), "parse corporate mock") {
		t.Fatalf("malformed HR snapshot: %v", err)
	}
	if err := os.WriteFile(path, []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_PORT", "invalid-port")
	if err := Run(); err == nil || !strings.Contains(err.Error(), "connect to PostgreSQL") || !strings.Contains(err.Error(), "invalid DB_PORT") {
		t.Fatalf("invalid PostgreSQL configuration: %v", err)
	}
}

func TestRunClosesStartupWhenHTTPPortUnavailable(t *testing.T) {
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
	defer db.Close()
	var version int
	if err := db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Skipf("port 8080 unavailable for bind-failure test: %v", err)
	}
	defer listener.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "corporate_employees.json"), []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", dir)
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long")
	if err := Run(); err == nil || !strings.Contains(err.Error(), "HTTP server") {
		t.Fatalf("occupied port startup error: %v", err)
	}
}
