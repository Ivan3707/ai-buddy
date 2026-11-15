package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/migrations"
)

func TestReadinessDatabaseUnavailable(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://127.0.0.1:1/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = NewReadinessRepository(db, migrations.Versions()).Check(ctx)
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("expected ErrDatabaseUnavailable, got %v", err)
	}
}

func TestReadinessPreparationStates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if name := strings.TrimPrefix(u.Path, "/"); !strings.HasPrefix(name, "ai_buddy_verify_") {
		t.Fatalf("unsafe test database %q", name)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	versions := migrations.Versions()
	cases := []struct {
		name          string
		applied       []int64
		createCatalog bool
		catalogRows   bool
		expected      error
	}{
		{name: "schema not migrated", expected: ErrSchemaNotMigrated},
		{name: "schema outdated", applied: versions[:len(versions)-1], createCatalog: true, catalogRows: true, expected: ErrSchemaOutdated},
		{name: "catalog table absent", applied: versions, expected: ErrCatalogEmpty},
		{name: "catalog empty", applied: versions, createCatalog: true, expected: ErrCatalogEmpty},
		{name: "fully ready", applied: versions, createCatalog: true, catalogRows: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := readinessSchema(t, db)
			if tc.applied != nil {
				if _, err := conn.ExecContext(context.Background(), `CREATE TABLE goose_db_version(version_id bigint NOT NULL,is_applied boolean NOT NULL)`); err != nil {
					t.Fatal(err)
				}
				for _, version := range tc.applied {
					if _, err := conn.ExecContext(context.Background(), `INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`, version); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.createCatalog {
				if _, err := conn.ExecContext(context.Background(), `CREATE TABLE checklist_items(id bigint PRIMARY KEY)`); err != nil {
					t.Fatal(err)
				}
				if tc.catalogRows {
					if _, err := conn.ExecContext(context.Background(), `INSERT INTO checklist_items(id) VALUES(1)`); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := NewReadinessRepository(conn, versions).Check(context.Background())
			if tc.expected == nil && err != nil {
				t.Fatalf("ready check: %v", err)
			}
			if tc.expected != nil && !errors.Is(err, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, err)
			}
		})
	}
}

func readinessSchema(t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()
	ctx := context.Background()
	schema := "readiness_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %s`, schema)); err != nil {
		t.Fatal(err)
	}
	return conn
}
