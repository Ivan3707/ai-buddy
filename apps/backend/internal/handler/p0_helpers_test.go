package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

func openP0TestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	var name string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "ai_buddy_test" && !strings.HasPrefix(name, "ai_buddy_verify_") {
		t.Fatalf("unsafe test database %q", name)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("expected Goose v5: version=%d err=%v", version, err)
	}
	return db, ctx
}

func createP0User(t *testing.T, db *sql.DB, ctx context.Context, role model.UserRole) *model.User {
	t.Helper()
	user := &model.User{Email: "p0-" + uuid.NewString() + "@example.test", PasswordHash: "test-hash", Role: role}
	if role == model.RoleIntern {
		profession := model.ProfessionBackend
		user.Profession = &profession
	}
	if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID); err != nil {
			t.Errorf("cleanup P0 user: %v", err)
		}
	})
	return user
}

func p0Token(t *testing.T, jwt *security.JWTService, user *model.User) string {
	t.Helper()
	token, err := jwt.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func p0Call(t *testing.T, router http.Handler, method, path, token string, payload any, want int, out any) {
	t.Helper()
	var body string
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = string(data)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: status %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
}
