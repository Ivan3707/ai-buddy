package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
)

var runIDPattern = regexp.MustCompile(`^mixed-[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$`)

const ownerMatch = `u.id = record.user_id
 AND u.email = 'load.intern.' || lpad(substring(record.title from '^load-vu-([1-9][0-9]{0,2})-'), 3, '0') || '@example.invalid'
 AND record.title ~ ('^load-vu-([1-9][0-9]{0,2})-' || $1 || '-[0-9]+$')`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	runID := flag.String("run-id", "", "exact mixed run ID")
	apply := flag.Bool("apply", false, "delete matching rows after count verification")
	expectNotes := flag.Int64("expect-notes", -1, "expected note count from dry-run")
	expectEvents := flag.Int64("expect-events", -1, "expected event count from dry-run")
	flag.Parse()
	if !runIDPattern.MatchString(*runID) {
		return fmt.Errorf("invalid run ID %q; expected mixed-YYYYMMDD-HHMMSS-8hex", *runID)
	}
	if *apply && (*expectNotes < 0 || *expectEvents < 0) {
		return fmt.Errorf("--apply requires nonnegative --expect-notes and --expect-events")
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actualDB string
	if err := tx.QueryRowContext(ctx, "SELECT current_database()").Scan(&actualDB); err != nil {
		return err
	}
	if actualDB != "ai_buddy_load" {
		return fmt.Errorf("refusing cleanup in database %q", actualDB)
	}
	if *apply {
		// Block concurrent writes while checking counts and deleting this run.
		if _, err := tx.ExecContext(ctx, "LOCK TABLE notes, calendar_events IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
	}
	notes, err := count(ctx, tx, "notes", "content = 'Synthetic k6 note'", *runID)
	if err != nil {
		return err
	}
	events, err := count(ctx, tx, "calendar_events", "description = 'Synthetic k6 event'", *runID)
	if err != nil {
		return err
	}
	if !*apply {
		fmt.Printf("DRY RUN database=%s run_id=%s notes=%d events=%d; no rows deleted\n", actualDB, *runID, notes, events)
		return nil
	}
	if notes != *expectNotes || events != *expectEvents {
		return fmt.Errorf("count changed: notes=%d expected=%d, events=%d expected=%d; no rows deleted", notes, *expectNotes, events, *expectEvents)
	}
	notesDeleted, err := remove(ctx, tx, "notes", "content = 'Synthetic k6 note'", *runID)
	if err != nil {
		return err
	}
	eventsDeleted, err := remove(ctx, tx, "calendar_events", "description = 'Synthetic k6 event'", *runID)
	if err != nil {
		return err
	}
	if notesDeleted != notes || eventsDeleted != events {
		return fmt.Errorf("delete count mismatch; transaction rolled back")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("APPLIED database=%s run_id=%s notes=%d events=%d\n", actualDB, *runID, notesDeleted, eventsDeleted)
	return nil
}

// table and marker are fixed call-site constants, never user input.
func predicate(marker string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM users u WHERE %s) AND record.%s", ownerMatch, marker)
}

func count(ctx context.Context, tx *sql.Tx, table, marker, runID string) (int64, error) {
	query := fmt.Sprintf("SELECT count(*) FROM %s record WHERE %s", table, predicate(marker))
	var n int64
	err := tx.QueryRowContext(ctx, query, runID).Scan(&n)
	return n, err
}

func remove(ctx context.Context, tx *sql.Tx, table, marker, runID string) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s record WHERE %s", table, predicate(marker))
	result, err := tx.ExecContext(ctx, query, runID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func init() {
	// Ensure usage failures go to stderr even in one-off Docker runs.
	flag.CommandLine.SetOutput(os.Stderr)
}
