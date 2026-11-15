package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestSyncHRCLIIdempotentCredentials(t *testing.T) {
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
		t.Fatalf("unsafe test DB %q", name)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var version int
	if err := db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("test schema version=%d err=%v", version, err)
	}
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	password, _ := u.User.Password()
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_SSLMODE", "disable")
	id := "cli-" + uuid.NewString()
	email := id + "@example.test"
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE corporate_id=$1`, id) })
	snapshot := filepath.Join(t.TempDir(), "snapshot.json")
	data, err := json.Marshal([]service.CorporateEmployee{{ID: id, Email: email, Role: "buddy"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshot, data, 0600); err != nil {
		t.Fatal(err)
	}
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	for attempt := 1; attempt <= 2; attempt++ {
		credentialPath := filepath.Join(t.TempDir(), "credentials.json")
		flag.CommandLine = flag.NewFlagSet("sync-hr-test", flag.ContinueOnError)
		os.Args = []string{"sync-hr", "-snapshot", snapshot, "-credentials", credentialPath}
		if err := run(); err != nil {
			t.Fatalf("CLI run %d: %v", attempt, err)
		}
		written, err := os.ReadFile(credentialPath)
		if err != nil {
			t.Fatal(err)
		}
		var credentials []service.InitialCredential
		if err := json.Unmarshal(written, &credentials); err != nil {
			t.Fatalf("credentials JSON: %v", err)
		}
		want := 0
		if attempt == 1 {
			want = 1
		}
		if len(credentials) != want {
			t.Fatalf("run %d published %d credentials, want %d", attempt, len(credentials), want)
		}
		if attempt == 1 && (credentials[0].Email != email || credentials[0].Password == "") {
			t.Fatal("initial credential identity incomplete")
		}
	}
	var users int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM users WHERE corporate_id=$1`, id).Scan(&users); err != nil || users != 1 {
		t.Fatalf("idempotent users=%d err=%v", users, err)
	}
}

func TestSyncHRCLIDemoPasswordWithoutCredentialsFile(t *testing.T) {
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
		t.Fatalf("unsafe test DB %q", name)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	password, _ := u.User.Password()
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_SSLMODE", "disable")
	id := "cli-demo-" + uuid.NewString()
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE corporate_id=$1`, id) })
	snapshot := filepath.Join(t.TempDir(), "snapshot.json")
	data, err := json.Marshal([]service.CorporateEmployee{{ID: id, Email: id + "@example.test", Role: "buddy"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshot, data, 0600); err != nil {
		t.Fatal(err)
	}
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })

	flag.CommandLine = flag.NewFlagSet("sync-hr-test", flag.ContinueOnError)
	os.Args = []string{"sync-hr", "-snapshot", snapshot, "-demo-password", "short"}
	if err := run(); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("short demo password accepted: %v", err)
	}

	flag.CommandLine = flag.NewFlagSet("sync-hr-test", flag.ContinueOnError)
	t.Setenv("DEMO_PASSWORD", "cli-demo-password")
	os.Args = []string{"sync-hr", "-snapshot", snapshot}
	if err := run(); err != nil {
		t.Fatalf("CLI run with DEMO_PASSWORD: %v", err)
	}
	var hash string
	if err := db.QueryRowContext(context.Background(), `SELECT password_hash FROM users WHERE corporate_id=$1`, id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, err := security.VerifyPassword("cli-demo-password", hash); err != nil || !ok {
		t.Fatalf("account does not use the demo password: ok=%v err=%v", ok, err)
	}
}

func TestSyncHRCLIRequiresCredentialsOrDemoPassword(t *testing.T) {
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	t.Setenv("DEMO_PASSWORD", "")
	flag.CommandLine = flag.NewFlagSet("sync-hr-test", flag.ContinueOnError)
	os.Args = []string{"sync-hr"}
	if err := run(); err == nil || !strings.Contains(err.Error(), "-credentials is required") {
		t.Fatalf("run without credentials or demo password: %v", err)
	}
}
