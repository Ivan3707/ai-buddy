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
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

const testDemoPassword = "seed-demo-test-password"

// openDemoDB connects to the disposable integration database and points DB_* at it for run().
func openDemoDB(t *testing.T) *sql.DB {
	t.Helper()
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
	return db
}

// prepareDemo loads the checklist catalog (if the shared test database has none) and the demo HR snapshot,
// and removes everything it created when the test ends.
func prepareDemo(t *testing.T, db *sql.DB) []service.CorporateEmployee {
	t.Helper()
	ctx := context.Background()
	var catalogs int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklists`).Scan(&catalogs); err != nil {
		t.Fatal(err)
	}
	if catalogs == 0 {
		for _, name := range []string{"catalog_common_backend.sql", "catalog_frontend.sql", "catalog_ml.sql"} {
			script, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, string(script)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "data", "corporate_employees.json"))
	if err != nil {
		t.Fatal(err)
	}
	var employees []service.CorporateEmployee
	if err := json.Unmarshal(data, &employees); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(employees))
	for _, e := range employees {
		ids = append(ids, e.ID)
	}
	t.Cleanup(func() {
		cleanup := []string{
			`DELETE FROM checklist_item_submissions WHERE progress_id IN (SELECT p.id FROM user_checklist_progress p JOIN users u ON u.id = p.user_id WHERE u.corporate_id = ANY($1))`,
			`DELETE FROM task_comments WHERE author_id IN (SELECT id FROM users WHERE corporate_id = ANY($1))`,
			`DELETE FROM users WHERE corporate_id = ANY($1)`,
		}
		for _, q := range cleanup {
			if _, err := db.ExecContext(ctx, q, ids); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if catalogs == 0 {
			if _, err := db.ExecContext(ctx, `DELETE FROM checklists`); err != nil {
				t.Errorf("cleanup catalog: %v", err)
			}
		}
	})
	sync, err := service.NewHRSynchronizer(db).WithDemoPassword(testDemoPassword)
	if err != nil {
		t.Fatal(err)
	}
	created, err := sync.Sync(ctx, employees, func(credentials []service.InitialCredential) error {
		for _, c := range credentials {
			if c.Password != testDemoPassword {
				t.Errorf("credential for %s does not use the demo password", c.Email)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if created != len(employees) {
		t.Fatalf("created %d accounts, want %d", created, len(employees))
	}
	return employees
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestSeedDemoScenario(t *testing.T) {
	db := openDemoDB(t)
	prepareDemo(t, db)
	ctx := context.Background()
	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, moscow)

	seeded, err := Seed(ctx, db, now)
	if err != nil || !seeded {
		t.Fatalf("first seed: seeded=%v err=%v", seeded, err)
	}

	// Every demo account signs in with the shared demo password.
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE email = $1`, mlEmail).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, err := security.VerifyPassword(testDemoPassword, hash); err != nil || !ok {
		t.Fatalf("demo password does not match: ok=%v err=%v", ok, err)
	}

	submissions := `SELECT count(*) FROM checklist_item_submissions s
		JOIN user_checklist_progress p ON p.id = s.progress_id JOIN users u ON u.id = p.user_id
		WHERE u.email = $1 AND s.status = $2`
	if n := count(t, db, submissions, mlEmail, "pending"); n != 1 {
		t.Errorf("ML intern pending submissions = %d, want 1", n)
	}
	if n := count(t, db, submissions, backendEmail, "rejected"); n != 1 {
		t.Errorf("backend intern rejected submissions = %d, want 1", n)
	}
	if n := count(t, db, submissions, frontendEmail, "pending"); n != 1 {
		t.Errorf("frontend intern pending submissions = %d, want 1", n)
	}
	if n := count(t, db, submissions, frontendEmail, "approved"); n < 3 {
		t.Errorf("frontend intern approved submissions = %d, want at least 3", n)
	}

	// The frontend intern finished the whole common checklist: the feedback form is available.
	open := `SELECT count(*) FROM user_checklist_progress p JOIN checklists c ON c.id = p.checklist_id
		JOIN users u ON u.id = p.user_id
		WHERE u.email = $1 AND c.type = 'common' AND p.hidden_at IS NULL AND p.status NOT IN ('completed', 'verified')`
	if n := count(t, db, open, frontendEmail); n != 0 {
		t.Errorf("frontend intern has %d unfinished common tasks, want 0", n)
	}

	// The ML intern's current task started three days ago, so the hint worker treats it as overdue.
	var started time.Time
	if err := db.QueryRowContext(ctx, `SELECT p.activated_at FROM user_checklist_progress p JOIN checklists c ON c.id = p.checklist_id
		JOIN users u ON u.id = p.user_id WHERE u.email = $1 AND c.type = 'common' AND p.status = 'active'`, mlEmail).Scan(&started); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(-72 * time.Hour); !started.Equal(want) {
		t.Errorf("ML intern task started at %v, want %v", started, want)
	}

	requests := `SELECT count(*) FROM service_requests r JOIN users u ON u.id = r.user_id WHERE u.email = $1 AND r.type = $2 AND r.status = 'pending_approval'`
	if n := count(t, db, requests, mlEmail, "access_request"); n != 1 {
		t.Errorf("ML intern access requests = %d, want 1", n)
	}
	if n := count(t, db, requests, backendEmail, "leave_request"); n != 1 {
		t.Errorf("backend intern leave requests = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM mentor_questions q JOIN users u ON u.id = q.intern_id WHERE u.email = $1 AND q.status = 'open'`, mlEmail); n != 1 {
		t.Errorf("open mentor questions = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM notes n JOIN users u ON u.id = n.user_id WHERE u.email = $1`, mlEmail); n != 2 {
		t.Errorf("ML intern notes = %d, want 2", n)
	}
	var standup time.Time
	if err := db.QueryRowContext(ctx, `SELECT e.starts_at FROM calendar_events e JOIN users u ON u.id = e.user_id
		WHERE u.email = $1 AND e.title = 'Стендап команды'`, mlEmail).Scan(&standup); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 17, 10, 0, 0, 0, moscow); !standup.Equal(want) {
		t.Errorf("standup starts at %v, want %v (Moscow time)", standup, want)
	}
	if n := count(t, db, `SELECT count(*) FROM calendar_events e JOIN users u ON u.id = e.user_id WHERE u.email IN ($1, $2)`, buddyEmail, teamleadEmail); n != 2 {
		t.Errorf("staff events = %d, want 2", n)
	}

	// A second run must not touch a database that is already in use.
	seeded, err = Seed(ctx, db, now)
	if err != nil || seeded {
		t.Fatalf("second seed: seeded=%v err=%v, want skipped", seeded, err)
	}
	if n := count(t, db, `SELECT count(*) FROM notes n JOIN users u ON u.id = n.user_id WHERE u.email = $1`, mlEmail); n != 2 {
		t.Errorf("second run changed notes: %d", n)
	}

	// The CLI wrapper reports the skip and succeeds.
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	if err := run(); err != nil {
		t.Fatalf("CLI run on a seeded database: %v", err)
	}
}

func TestSeedDemoRequiresUsers(t *testing.T) {
	db := openDemoDB(t)
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, buddyEmail).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Skip("demo users already exist in the shared test database")
	}
	if _, err := Seed(context.Background(), db, time.Now()); err == nil || !strings.Contains(err.Error(), "run sync-hr first") {
		t.Fatalf("seed without demo users: %v", err)
	}
}

func TestRunRequiresDatabaseName(t *testing.T) {
	t.Setenv("DB_NAME", "")
	if err := run(); err == nil || !strings.Contains(err.Error(), "DB_NAME") {
		t.Fatalf("run without DB_NAME: %v", err)
	}
}
