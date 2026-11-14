package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	if err = db.QueryRowContext(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 1 {
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

	t.Run("concurrent refresh replay revokes every token", func(t *testing.T) {
		initialAccess, initialRefresh := login()
		type result struct {
			code int
			body []byte
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		var workers sync.WaitGroup
		for i := 0; i < 2; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				code, body := call("POST", "/api/auth/refresh", `{"refresh_token":"`+initialRefresh+`"}`, "")
				results <- result{code, body}
			}()
		}
		close(start)
		workers.Wait()
		close(results)
		var newAccess, newRefresh string
		var accepted, rejected int
		for got := range results {
			switch got.code {
			case 200:
				accepted++
				var tokens struct {
					AccessToken  string `json:"access_token"`
					RefreshToken string `json:"refresh_token"`
				}
				if e := json.Unmarshal(got.body, &tokens); e != nil {
					t.Fatal(e)
				}
				newAccess, newRefresh = tokens.AccessToken, tokens.RefreshToken
			case 401:
				rejected++
			default:
				t.Fatalf("unexpected concurrent refresh status %d: %s", got.code, got.body)
			}
		}
		if accepted != 1 || rejected != 1 || newAccess == "" || newRefresh == "" {
			t.Fatalf("refresh outcomes: accepted=%d rejected=%d", accepted, rejected)
		}
		for _, token := range []string{initialAccess, newAccess} {
			if code, _ := call("GET", "/api/auth/me", "", token); code != 401 {
				t.Fatalf("replay left access token active: %d", code)
			}
		}
		for _, token := range []string{initialRefresh, newRefresh} {
			if code, _ := call("POST", "/api/auth/refresh", `{"refresh_token":"`+token+`"}`, ""); code != 401 {
				t.Fatalf("replay left refresh token active: %d", code)
			}
		}
		sessionID := sessionIDFromAccess(t, jwt, initialAccess)
		if active, e := sessions.IsActive(ctx, sessionID, user.ID); e != nil || active {
			t.Fatalf("session after replay: active=%v err=%v", active, e)
		}
	})

	t.Run("concurrent refresh and logout", func(t *testing.T) {
		initialAccess, initialRefresh := login()
		type result struct {
			kind string
			code int
			body []byte
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			code, body := call("POST", "/api/auth/refresh", `{"refresh_token":"`+initialRefresh+`"}`, "")
			results <- result{"refresh", code, body}
		}()
		go func() {
			defer workers.Done()
			<-start
			code, body := call("POST", "/api/auth/logout", `{"refresh_token":"`+initialRefresh+`"}`, initialAccess)
			results <- result{"logout", code, body}
		}()
		close(start)
		workers.Wait()
		close(results)
		var refreshResult, logoutResult result
		for got := range results {
			if got.kind == "refresh" {
				refreshResult = got
			} else {
				logoutResult = got
			}
		}
		sessionID := sessionIDFromAccess(t, jwt, initialAccess)
		if logoutResult.code == 204 {
			if refreshResult.code != 401 {
				t.Fatalf("logout won: refresh=%d", refreshResult.code)
			}
		} else if refreshResult.code == 200 && logoutResult.code == 400 {
			var tokens struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
			}
			if e := json.Unmarshal(refreshResult.body, &tokens); e != nil || tokens.AccessToken == "" || tokens.RefreshToken == "" {
				t.Fatalf("rotated tokens: %v", e)
			}
			if code, _ := call("GET", "/api/auth/me", "", tokens.AccessToken); code != 200 {
				t.Fatalf("rotated access unavailable before logout: %d", code)
			}
			if code, _ := call("POST", "/api/auth/logout", `{"refresh_token":"`+tokens.RefreshToken+`"}`, tokens.AccessToken); code != 204 {
				t.Fatalf("logout with rotated token: %d", code)
			}
			if code, _ := call("GET", "/api/auth/me", "", tokens.AccessToken); code != 401 {
				t.Fatalf("rotated access active after logout: %d", code)
			}
			if code, _ := call("POST", "/api/auth/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`, ""); code != 401 {
				t.Fatalf("rotated refresh active after logout: %d", code)
			}
		} else {
			t.Fatalf("unexpected race outcomes: refresh=%d logout=%d", refreshResult.code, logoutResult.code)
		}
		if code, _ := call("GET", "/api/auth/me", "", initialAccess); code != 401 {
			t.Fatalf("initial access active after race: %d", code)
		}
		if code, _ := call("POST", "/api/auth/refresh", `{"refresh_token":"`+initialRefresh+`"}`, ""); code != 401 {
			t.Fatalf("initial refresh active after race: %d", code)
		}
		if active, e := sessions.IsActive(ctx, sessionID, user.ID); e != nil || active {
			t.Fatalf("session after race: active=%v err=%v", active, e)
		}
	})
}

func sessionIDFromAccess(t *testing.T, jwt *security.JWTService, access string) uuid.UUID {
	t.Helper()
	_, id, err := jwt.ValidateSessionAccessToken(access)
	if err != nil || id == uuid.Nil {
		t.Fatalf("invalid session access token: %v", err)
	}
	return id
}
