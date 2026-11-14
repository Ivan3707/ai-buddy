package database

import (
	"os"
	"testing"
)

func TestPostgresConfigPreservesPassword(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
	}{
		{name: "spaces", password: "contains spaces"},
		{name: "backslash", password: `back\slash`},
		{name: "special_chars", password: `punctuation'"=:#@/?!&`},
		{name: "leading_trailing_spaces", password: " leading and trailing "},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DB_HOST", "127.0.0.1")
			t.Setenv("DB_PORT", "15433")
			t.Setenv("DB_USER", "test_user")
			t.Setenv("DB_PASSWORD", test.password)
			t.Setenv("DB_NAME", "test_db")
			t.Setenv("DB_SSLMODE", "disable")
			t.Setenv("PGPASSWORD", "different_environment_password")
			config, err := postgresConfig()
			if err != nil {
				t.Fatalf("postgresConfig: %v", err)
			}
			if config.Host != "127.0.0.1" || config.Port != 15433 || config.User != "test_user" || config.Database != "test_db" || config.Password != test.password || config.TLSConfig != nil {
				t.Fatal("database configuration did not preserve explicit environment values")
			}
		})
	}
}

func TestPostgresConfigSSLModeFallback(t *testing.T) {
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "15433")
	t.Setenv("DB_SSLMODE", "prefer")
	config, err := postgresConfig()
	if err != nil {
		t.Fatalf("postgresConfig: %v", err)
	}
	for _, fallback := range config.Fallbacks {
		if fallback.Host != config.Host || fallback.Port != config.Port {
			t.Fatal("SSL fallback does not use the configured host and port")
		}
	}
}

func TestOpenPostgresLive(t *testing.T) {
	if os.Getenv("VERIFY_DATABASE_CONNECTION") != "1" {
		t.Skip("set VERIFY_DATABASE_CONNECTION=1 with DB_* variables to check a running PostgreSQL")
	}
	db, err := OpenPostgres()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("read-only PostgreSQL check failed: %v", err)
	}
}
