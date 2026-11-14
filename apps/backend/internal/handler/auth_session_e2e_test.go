package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
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
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestAuthSessionsHTTPIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var dbName string
	var version int
	if err = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	if dbName != "ai_buddy_test" && !strings.HasPrefix(dbName, "ai_buddy_verify_") {
		t.Fatalf("unsafe DB %q", dbName)
	}
	if err = db.QueryRowContext(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 3 {
		t.Fatalf("expected Goose v1: %d %v", version, err)
	}
	hash, err := security.HashPassword("strong-password-123")
	if err != nil {
		t.Fatal(err)
	}
	first, last, telegram := "Test", "Buddy", "test_buddy"
	user := &model.User{Email: "session-http-" + uuid.NewString() + "@example.com", PasswordHash: hash, Role: model.RoleBuddy, FirstName: &first, LastName: &last, TelegramUsername: &telegram}
	users := repository.NewUserRepository(db)
	if err = users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, e := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID); e != nil {
			t.Errorf("cleanup session user: %v", e)
		}
	})
	sessions := repository.NewSessionRepository(db)
	jwt, err := security.NewJWTService("12345678901234567890123456789012", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auth := service.NewAuthServiceWithSessions(users, jwt, sessions)
	handler := handler.NewRouter("", handler.Dependencies{Auth: auth, Users: users, JWT: jwt, Sessions: auth, SessionCheck: sessions})
	call := func(method, path, body, token string) (int, []byte) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code, w.Body.Bytes()
	}
	login := func() (string, string) {
		t.Helper()
		code, body := call("POST", "/api/auth/login", `{"email":"`+user.Email+`","password":"strong-password-123"}`, "")
		if code != 200 {
			t.Fatalf("login status %d", code)
		}
		var value struct {
			AccessToken  string     `json:"access_token"`
			RefreshToken string     `json:"refresh_token"`
			User         publicUser `json:"user"`
		}
		if err = json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if value.User.FirstName == nil || *value.User.FirstName != first || value.User.LastName == nil || *value.User.LastName != last || value.User.TelegramUsername == nil || *value.User.TelegramUsername != telegram {
			t.Fatal("profile fields missing from login")
		}
		return value.AccessToken, value.RefreshToken
	}
	access, refresh := login()
	if code, _ := call("GET", "/api/auth/me", "", access); code != 200 {
		t.Fatalf("active session access: %d", code)
	}
	code, body := call("POST", "/api/auth/refresh", `{"refresh_token":"`+refresh+`"}`, "")
	if code != 200 {
		t.Fatalf("refresh status %d", code)
	}
	var rotated struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err = json.Unmarshal(body, &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == refresh || rotated.AccessToken == "" {
		t.Fatal("refresh did not rotate")
	}
	if code, _ := call("POST", "/api/auth/refresh", `{"refresh_token":"`+refresh+`"}`, ""); code != 401 {
		t.Fatalf("replay status %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", rotated.AccessToken); code != 401 {
		t.Fatalf("replay must revoke access, got %d", code)
	}
	access, refresh = login()
	parts := strings.Split(refresh, ".")
	invalidRefresh := parts[0] + "." + strings.Repeat("x", 43)
	if code, _ := call("POST", "/api/auth/logout", `{"refresh_token":"`+invalidRefresh+`"}`, access); code != 400 {
		t.Fatalf("logout with invalid refresh secret: %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", access); code != 200 {
		t.Fatalf("invalid logout must not revoke access, got %d", code)
	}
	if code, _ := call("POST", "/api/auth/logout", `{"refresh_token":"`+refresh+`"}`, access); code != 204 {
		t.Fatalf("logout status %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", access); code != 401 {
		t.Fatalf("logout must revoke access, got %d", code)
	}
	if code, _ := call("POST", "/api/auth/refresh", `{"refresh_token":"`+refresh+`"}`, ""); code != 401 {
		t.Fatalf("logout must revoke refresh, got %d", code)
	}

	// ---- link: a second client gets its OWN session; refresh chains and logouts are independent
	siteAccess, siteRefresh := login()
	code, body = call("POST", "/api/auth/link", `{"refresh_token":"`+siteRefresh+`"}`, "")
	if code != 201 {
		t.Fatalf("link status %d: %s", code, body)
	}
	var linked struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err = json.Unmarshal(body, &linked); err != nil {
		t.Fatal(err)
	}
	if linked.RefreshToken == "" || linked.RefreshToken == siteRefresh || strings.Split(linked.RefreshToken, ".")[0] == strings.Split(siteRefresh, ".")[0] {
		t.Fatal("link must create a different session")
	}
	if code, _ := call("GET", "/api/auth/me", "", linked.AccessToken); code != 200 {
		t.Fatalf("linked access: %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", siteAccess); code != 200 {
		t.Fatalf("link must not touch the caller's session: %d", code)
	}
	// the caller's refresh was not rotated: it still works once (and rotates then), the linked chain is unaffected
	code, body = call("POST", "/api/auth/refresh", `{"refresh_token":"`+siteRefresh+`"}`, "")
	if code != 200 {
		t.Fatalf("caller refresh after link: %d", code)
	}
	var siteNext struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err = json.Unmarshal(body, &siteNext); err != nil {
		t.Fatal(err)
	}
	code, body = call("POST", "/api/auth/refresh", `{"refresh_token":"`+linked.RefreshToken+`"}`, "")
	if code != 200 {
		t.Fatalf("linked refresh: %d", code)
	}
	if err = json.Unmarshal(body, &linked); err != nil {
		t.Fatal(err)
	}
	// replaying a rotated token cannot open a session; garbage cannot either
	if code, _ := call("POST", "/api/auth/link", `{"refresh_token":"`+siteRefresh+`"}`, ""); code != 401 {
		t.Fatalf("link with a rotated token: %d", code)
	}
	if code, _ := call("POST", "/api/auth/link", `{"refresh_token":"nope"}`, ""); code != 401 {
		t.Fatalf("link with garbage: %d", code)
	}
	// logging one client out leaves the other signed in
	if code, _ := call("POST", "/api/auth/logout", `{"refresh_token":"`+linked.RefreshToken+`"}`, linked.AccessToken); code != 204 {
		t.Fatalf("linked logout: %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", linked.AccessToken); code != 401 {
		t.Fatalf("linked session must be revoked: %d", code)
	}
	if code, _ := call("GET", "/api/auth/me", "", siteNext.AccessToken); code != 200 {
		t.Fatalf("the other client must stay signed in: %d", code)
	}
}
