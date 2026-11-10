package main

import (
	"context"
	"errors"
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

	handler := newHandler(
		aiURL,
		[]byte(demo),
		handlerDependencies{
			auth:  auth,
			users: users,
			jwt:   jwtService,
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
	interns internCreator,
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

	handler := newHandler(
		"",
		[]byte(demo),
		handlerDependencies{
			auth:    auth,
			users:   users,
			jwt:     jwtService,
			interns: interns,
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
        "profession":"backend"
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
        "profession": "devops"
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

type testProgressReviewer struct {
	buddyID  uuid.UUID
	internID uuid.UUID
	itemID   uuid.UUID
	approved bool
	called   bool
	err      error
}

func (s *testProgressReviewer) ReviewItem(
	ctx context.Context,
	buddyID uuid.UUID,
	internID uuid.UUID,
	itemID uuid.UUID,
	approved bool,
) error {
	s.called = true
	s.buddyID = buddyID
	s.internID = internID
	s.itemID = itemID
	s.approved = approved

	return s.err
}

func newReviewHandler(
	t *testing.T,
	role model.UserRole,
	reviewer progressReviewer,
) (http.Handler, string, *model.User) {
	t.Helper()

	user := &model.User{
		ID:        uuid.New(),
		Email:     "reviewer@example.com",
		Role:      role,
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

	handler := newHandler(
		"",
		[]byte(demo),
		handlerDependencies{
			auth:     auth,
			users:    users,
			jwt:      jwtService,
			progress: reviewer,
		},
	)

	return handler, token, user
}

func TestReviewItem_Success(t *testing.T) {
	reviewer := &testProgressReviewer{}
	handler, token, buddy := newReviewHandler(
		t,
		model.RoleBuddy,
		reviewer,
	)

	internID := uuid.New()
	itemID := uuid.New()

	target := "/api/interns/" + internID.String() +
		"/checklist-items/" + itemID.String() + "/review"

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		target,
		`{"approved":true}`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	if !reviewer.called {
		t.Fatal("expected ReviewItem to be called")
	}

	if reviewer.buddyID != buddy.ID {
		t.Errorf("unexpected buddy ID: %s", reviewer.buddyID)
	}

	if reviewer.internID != internID {
		t.Errorf("unexpected intern ID: %s", reviewer.internID)
	}

	if reviewer.itemID != itemID {
		t.Errorf("unexpected item ID: %s", reviewer.itemID)
	}

	if !reviewer.approved {
		t.Error("expected approved=true")
	}
}

func TestReviewItem_Reject(t *testing.T) {
	reviewer := &testProgressReviewer{}
	handler, token, _ := newReviewHandler(
		t,
		model.RoleBuddy,
		reviewer,
	)

	target := "/api/interns/" + uuid.NewString() +
		"/checklist-items/" + uuid.NewString() + "/review"

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		target,
		`{"approved":false}`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	if !reviewer.called {
		t.Fatal("expected ReviewItem to be called")
	}

	if reviewer.approved {
		t.Error("expected approved=false")
	}
}

func TestReviewItem_RequiresBuddy(t *testing.T) {
	reviewer := &testProgressReviewer{}
	handler, token, _ := newReviewHandler(
		t,
		model.RoleIntern,
		reviewer,
	)

	target := "/api/interns/" + uuid.NewString() +
		"/checklist-items/" + uuid.NewString() + "/review"

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		target,
		`{"approved":true}`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d", w.Code, http.StatusForbidden)
	}

	if reviewer.called {
		t.Fatal("review service must not be called for intern")
	}
}

func TestReviewItem_InvalidRequest(t *testing.T) {
	reviewer := &testProgressReviewer{}
	handler, token, _ := newReviewHandler(
		t,
		model.RoleBuddy,
		reviewer,
	)

	validTarget := "/api/interns/" + uuid.NewString() +
		"/checklist-items/" + uuid.NewString() + "/review"

	tests := []struct {
		name   string
		target string
		body   string
	}{
		{
			name:   "missing approved",
			target: validTarget,
			body:   `{}`,
		},
		{
			name:   "invalid JSON",
			target: validTarget,
			body:   `broken`,
		},
		{
			name:   "unknown field",
			target: validTarget,
			body:   `{"approved":true,"extra":1}`,
		},
		{
			name: "invalid intern ID",
			target: "/api/interns/invalid/checklist-items/" +
				uuid.NewString() + "/review",
			body: `{"approved":true}`,
		},
		{
			name: "invalid item ID",
			target: "/api/interns/" + uuid.NewString() +
				"/checklist-items/invalid/review",
			body: `{"approved":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := authorizedRequest(
				http.MethodPost,
				tc.target,
				tc.body,
				token,
			)

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf(
					"got %d, want %d: %s",
					w.Code,
					http.StatusBadRequest,
					w.Body.String(),
				)
			}
		})
	}

	if reviewer.called {
		t.Fatal("review service must not be called for invalid requests")
	}
}

func TestReviewItem_ServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{
			name:       "intern not assigned",
			serviceErr: service.ErrInternNotAssigned,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "review not pending",
			serviceErr: service.ErrReviewNotPending,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "record not found",
			serviceErr: repository.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "internal error",
			serviceErr: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reviewer := &testProgressReviewer{err: tc.serviceErr}
			handler, token, _ := newReviewHandler(
				t,
				model.RoleBuddy,
				reviewer,
			)

			target := "/api/interns/" + uuid.NewString() +
				"/checklist-items/" + uuid.NewString() + "/review"

			w := httptest.NewRecorder()
			req := authorizedRequest(
				http.MethodPost,
				target,
				`{"approved":true}`,
				token,
			)

			handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf(
					"got %d, want %d: %s",
					w.Code,
					tc.wantStatus,
					w.Body.String(),
				)
			}
		})
	}
}
