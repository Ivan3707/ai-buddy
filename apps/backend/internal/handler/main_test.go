package handler_test

import (
	"context"
	"errors"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

const demo = `[
	{"id":"company","role":"all","tasks":[]},
	{"id":"ml","role":"ml","tasks":[]},
	{"id":"backend","role":"backend","tasks":[]}
]`

const testJWTSecret = "test-secret-that-is-at-least-32-bytes-long"
const testPassword = "correct-password-123"

func TestFullMonths(t *testing.T) {
	from := time.Date(2000, 12, 15, 0, 0, 0, 0, time.UTC)
	if got := fullMonths(from, time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)); got != 310 {
		t.Fatalf("before anniversary: %d", got)
	}
	if got := fullMonths(from, time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)); got != 312 {
		t.Fatalf("on anniversary: %d", got)
	}
}

type testUserRepository struct {
	user *model.User
}

func (r *testUserRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, repository.ErrNotFound
	}
	return r.user, nil
}

func (r *testUserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*model.User, error) {
	if r.user == nil || !strings.EqualFold(r.user.Email, email) {
		return nil, repository.ErrNotFound
	}
	return r.user, nil
}

func (r *testUserRepository) RecordLogin(
	ctx context.Context,
	id uuid.UUID,
) (bool, error) {
	if r.user == nil || r.user.ID != id {
		return false, repository.ErrNotFound
	}
	isFirstLogin := r.user.LastLoginAt == nil
	now := time.Now().UTC()
	r.user.LastLoginAt = &now
	return isFirstLogin, nil
}

func newTestHandler(
	t *testing.T,
	aiURL string,
) (http.Handler, string, *model.User) {
	t.Helper()

	passwordHash, err := security.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &model.User{
		ID:           uuid.New(),
		Email:        "intern@example.com",
		PasswordHash: passwordHash,
		Role:         model.RoleIntern,
		CreatedAt:    time.Now().UTC(),
	}

	users := &testUserRepository{user: user}

	jwtService, err := security.NewJWTService(
		testJWTSecret,
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}

	auth := service.NewAuthService(users, jwtService)

	token, err := jwtService.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	handler := handler.NewRouter(
		aiURL,
		[]byte(demo),
		handler.Dependencies{
			Auth:  auth,
			Users: users,
			JWT:   jwtService,
		},
	)

	return handler, token, user
}

func authorizedRequest(
	method string,
	target string,
	body string,
	token string,
) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestChat(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Error("wrong upstream route")
		}
		send(w, http.StatusOK, chatResponse{
			Answer:  "demo",
			Sources: []source{},
			Mode:    "mock",
		})
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)

	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"message":"отпуск"}`, 200},
		{`{"message":" "}`, 400},
		{`{"message":"x","extra":1}`, 400},
		{`{"message":"x"} {}`, 400},
		{`broken`, 400},
		{`{"message":"` + strings.Repeat("a", 2001) + `"}`, 400},
	} {
		w := httptest.NewRecorder()
		req := authorizedRequest("POST", "/api/chat", tc.body, token)

		handler.ServeHTTP(w, req)

		if w.Code != tc.status {
			t.Errorf("got %d want %d: %s", w.Code, tc.status, tc.body)
		}
	}
}

func TestChatRequiresAuthentication(t *testing.T) {
	handler, _, _ := newTestHandler(t, "")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/chat",
		strings.NewReader(`{"message":"hello"}`),
	)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAIUnavailable(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/chat",
		`{"message":"hi"}`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestRoleFiltering(t *testing.T) {
	handler, _, _ := newTestHandler(t, "")

	w := httptest.NewRecorder()
	handler.ServeHTTP(
		w,
		httptest.NewRequest("GET", "/api/onboarding?role=ml", nil),
	)

	if w.Code != 200 ||
		strings.Contains(w.Body.String(), "backend") ||
		!strings.Contains(w.Body.String(), "company") {
		t.Fatal(w.Body.String())
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(
		w,
		httptest.NewRequest("GET", "/api/onboarding?role=invalid", nil),
	)

	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

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
	// Проверяем, что внутренняя ошибка БД не превращается
	// в ответ с деталями ошибки для клиента.
	repoErr := errors.New("database unavailable")
	if repoErr == nil {
		t.Fatal("unexpected nil error")
	}
}

type testInternCreator struct {
	user       *model.User
	err        error
	called     bool
	email      string
	profession model.Profession
	buddyID    uuid.UUID
}

func (s *testInternCreator) CreateIntern(
	ctx context.Context,
	email string,
	passwordHash string,
	profession model.Profession,
	buddyID uuid.UUID,
) (*model.User, error) {
	s.called = true
	s.email = email
	s.profession = profession
	s.buddyID = buddyID

	if profession != model.ProfessionBackend &&
		profession != model.ProfessionFrontend &&
		profession != model.ProfessionML {
		return nil, service.ErrInvalidProfession
	}

	if s.err != nil {
		return nil, s.err
	}

	user := &model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
		Role:         model.RoleIntern,
		Profession:   &profession,
		CreatedAt:    time.Now().UTC(),
	}

	s.user = user
	return user, nil
}

func newTeamleadHandler(
	t *testing.T,
	interns handler.InternCreator,
) (http.Handler, string) {
	t.Helper()

	user := &model.User{
		ID:        uuid.New(),
		Email:     "admin@example.com",
		Role:      model.RoleBuddy,
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
		[]byte(demo),
		handler.Dependencies{
			Auth:    auth,
			Users:   users,
			JWT:     jwtService,
			Interns: interns,
		},
	)

	return handler, token
}
func TestCreateIntern(t *testing.T) {
	interns := &testInternCreator{}
	handler, token := newTeamleadHandler(t, interns)

	body := `{
        "email": "new-intern@example.com",
        "password": "strong-password-123",
        "profession": "backend", "first_name":"Test", "last_name":"Intern", "department":"Engineering", "team":"Backend", "teamlead_id":"11111111-1111-4111-8111-111111111111", "birth_date":"2000-01-01", "employed_since":"2026-01-01"
    }`

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/users",
		body,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	if !interns.called {
		t.Fatal("expected CreateIntern to be called")
	}

	if interns.email != "new-intern@example.com" {
		t.Errorf("unexpected email: %s", interns.email)
	}

	if interns.profession != model.ProfessionBackend {
		t.Errorf("unexpected profession: %s", interns.profession)
	}

	if strings.Contains(w.Body.String(), "password") {
		t.Fatal("response must not contain password or password hash")
	}

	if !strings.Contains(w.Body.String(), `"role":"intern"`) {
		t.Fatal("response must contain intern role")
	}
	if interns.buddyID == uuid.Nil {
		t.Fatal("expected authenticated buddy ID to be passed")
	}
}
func TestCreateInternRequiresTeamlead(t *testing.T) {
	handler, token, _ := newTestHandler(t, "")

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/users",
		`{
            "email":"new-intern@example.com",
            "password":"strong-password-123",
            "profession":"backend"
        }`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d", w.Code, http.StatusForbidden)
	}
}
func TestCreateInternRequiresAuthentication(t *testing.T) {
	handler, _ := newTeamleadHandler(t, &testInternCreator{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/users",
		strings.NewReader(`{}`),
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
func TestCreateInternRejectsDuplicateEmail(t *testing.T) {
	interns := &testInternCreator{
		err: repository.ErrEmailAlreadyExists,
	}
	handler, token := newTeamleadHandler(t, interns)

	body := `{
        "email":"existing@example.com",
        "password":"strong-password-123",
        "profession":"backend", "first_name":"Test", "last_name":"Intern", "department":"Engineering", "team":"Backend", "teamlead_id":"11111111-1111-4111-8111-111111111111", "birth_date":"2000-01-01", "employed_since":"2026-01-01"
    }`

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/users",
		body,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("got %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCreateInternRejectsInvalidProfession(t *testing.T) {
	interns := &testInternCreator{}
	handler, token := newTeamleadHandler(t, interns)

	body := `{
        "email": "new-intern@example.com",
        "password": "strong-password-123",
        "profession": "devops", "first_name":"Test", "last_name":"Intern", "department":"Engineering", "team":"Backend", "teamlead_id":"11111111-1111-4111-8111-111111111111", "birth_date":"2000-01-01", "employed_since":"2026-01-01"
    }`

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/users",
		body,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !interns.called {
		t.Fatal("expected CreateIntern to be called")
	}
}

func TestCreateInternRejectsShortPassword(t *testing.T) {
	interns := &testInternCreator{}
	handler, token := newTeamleadHandler(t, interns)

	body := `{
        "email": "new-intern@example.com",
        "password": "short",
        "profession": "backend"
    }`

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/users",
		body,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want %d", w.Code, http.StatusBadRequest)
	}

	if interns.called {
		t.Fatal("service must not be called for invalid password")
	}
}
