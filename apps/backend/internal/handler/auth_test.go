package handler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestAuthLogin(t *testing.T) {
	handler, _, user := newTestHandler(t, "")

	body := `{"email":"intern@example.com","password":"` + testPassword + `"}`
	w := httptest.NewRecorder()

	handler.ServeHTTP(
		w,
		httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body)),
	)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), `"access_token"`) {
		t.Fatalf("access token missing: %s", w.Body.String())
	}

	if !strings.Contains(w.Body.String(), user.ID.String()) {
		t.Fatalf("user ID missing: %s", w.Body.String())
	}

	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("response must not contain password hash")
	}
	if !strings.Contains(w.Body.String(), `"is_first_login":true`) {
		t.Fatalf("expected first login flag: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(
		w,
		httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body)),
	)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"is_first_login":false`) {
		t.Fatalf("expected subsequent login flag: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthLoginRejectsInvalidCredentials(t *testing.T) {
	handler, _, _ := newTestHandler(t, "")

	body := `{"email":"intern@example.com","password":"wrong-password"}`
	w := httptest.NewRecorder()

	handler.ServeHTTP(
		w,
		httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body)),
	)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMe(t *testing.T) {
	handler, token, user := newTestHandler(t, "")

	w := httptest.NewRecorder()
	handler.ServeHTTP(
		w,
		authorizedRequest("GET", "/api/auth/me", "", token),
	)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), user.ID.String()) {
		t.Fatalf("user ID missing: %s", w.Body.String())
	}

	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("response must not contain password hash")
	}
}

func TestAuthMeRequiresAuthentication(t *testing.T) {
	handler, _, _ := newTestHandler(t, "")

	w := httptest.NewRecorder()
	handler.ServeHTTP(
		w,
		httptest.NewRequest("GET", "/api/auth/me", nil),
	)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthServiceRepositoryErrorIsNotExposed(t *testing.T) {
	repoErr := errors.New("database unavailable: secret connection details")
	users := &testUserRepository{getByEmailErr: repoErr}
	jwtService, err := security.NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}
	router := handler.NewRouter("", handler.Dependencies{
		Auth:  service.NewAuthService(users, jwtService),
		Users: users,
		JWT:   jwtService,
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(
		http.MethodPost,
		"/api/auth/login",
		strings.NewReader(`{"email":"intern@example.com","password":"`+testPassword+`"}`),
	))

	if users.getByEmailCalls != 1 {
		t.Fatalf("expected one repository lookup, got %d", users.getByEmailCalls)
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %q", got)
	}
	if got := w.Body.String(); got != "{\"error\":\"internal server error\"}\n" {
		t.Fatalf("unsafe or unexpected response: %q", got)
	}
}

func newTeamleadHandler(t *testing.T) (http.Handler, string) {
	t.Helper()

	user := &model.User{
		ID:        uuid.New(),
		Email:     "admin@example.com",
		Role:      model.RoleTeamlead,
		CreatedAt: time.Now().UTC(),
	}

	users := &testUserRepository{user: user}

	jwtService, err := security.NewJWTService(
		testJWTSecret,
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}

	token, err := jwtService.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	auth := service.NewAuthService(users, jwtService)

	handler := handler.NewRouter(
		"",
		handler.Dependencies{
			Auth:  auth,
			Users: users,
			JWT:   jwtService,
		},
	)

	return handler, token
}
func TestCreateInternHTTPRemoved(t *testing.T) {
	router, token := newTeamleadHandler(t)
	for _, candidate := range []string{"", token} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/users", `{}`, candidate))
		if w.Code != http.StatusNotFound {
			t.Fatalf("public creation: status=%d", w.Code)
		}
	}
}
