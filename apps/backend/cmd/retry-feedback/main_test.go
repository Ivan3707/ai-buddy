package main

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRetryFeedbackCLIRespectsUnavailableAnalyticsAndRetries(t *testing.T) {
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
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_SSLMODE", "disable")
	userID, checklistID, feedbackID := uuid.New(), uuid.New(), uuid.New()
	corpID := "retry-cli-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,corporate_id,email,password_hash,role,profession) VALUES($1,$2,$3,'test','intern','backend')`, userID, corpID, corpID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID) })
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists(id,name,type) VALUES($1,$2,'common')`, checklistID, "Retry CLI "+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM checklists WHERE id=$1`, checklistID) })
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_feedback(id,user_id,checklist_id,rating,comment) VALUES($1,$2,$3,5,'retry safely')`, feedbackID, userID, checklistID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM mock_analytics_feedback WHERE feedback_id=$1`, feedbackID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_feedback WHERE id=$1`, feedbackID)
	})
	t.Setenv("ANALYTICS_MOCK_UNAVAILABLE", "1")
	main()
	var status string
	if err := db.QueryRowContext(ctx, `SELECT delivery_status FROM checklist_feedback WHERE id=$1`, feedbackID).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("offline status=%q err=%v", status, err)
	}
	t.Setenv("ANALYTICS_MOCK_UNAVAILABLE", "0")
	main()
	var delivered int
	if err := db.QueryRowContext(ctx, `SELECT delivery_status FROM checklist_feedback WHERE id=$1`, feedbackID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("retry status=%q err=%v", status, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mock_analytics_feedback WHERE feedback_id=$1 AND corporate_id=$2`, feedbackID, corpID).Scan(&delivered); err != nil || delivered != 1 {
		t.Fatalf("analytics delivery count=%d err=%v", delivered, err)
	}
	main()
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mock_analytics_feedback WHERE feedback_id=$1`, feedbackID).Scan(&delivered); err != nil || delivered != 1 {
		t.Fatalf("repeated retry duplicated delivery: count=%d err=%v", delivered, err)
	}
}
