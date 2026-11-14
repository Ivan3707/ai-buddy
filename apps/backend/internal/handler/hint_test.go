package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type hintReaderStub struct {
	listFn func(context.Context, uuid.UUID) ([]repository.Hint, error)
	getFn  func(context.Context, uuid.UUID, uuid.UUID) (*repository.Hint, error)
	readFn func(context.Context, uuid.UUID, uuid.UUID) error
}

func (s *hintReaderStub) List(ctx context.Context, userID uuid.UUID) ([]repository.Hint, error) {
	return s.listFn(ctx, userID)
}

func (s *hintReaderStub) Get(ctx context.Context, userID, itemID uuid.UUID) (*repository.Hint, error) {
	return s.getFn(ctx, userID, itemID)
}

func (s *hintReaderStub) Read(ctx context.Context, userID, hintID uuid.UUID) error {
	return s.readFn(ctx, userID, hintID)
}

type hintUserRepositoryStub struct {
	user *model.User
}

func (s *hintUserRepositoryStub) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.User, error) {
	if s.user != nil && s.user.ID == id {
		return s.user, nil
	}
	return nil, repository.ErrNotFound
}

// newHintTestHandler создаёт HTTP-обработчик с настоящей
// JWT-аутентификацией и тестовыми зависимостями.
func newHintTestHandler(
	t *testing.T,
	user *model.User,
	hints HintReader,
) (http.Handler, string) {
	t.Helper()

	jwtService, err := security.NewJWTService(
		"test-secret-at-least-32-bytes-long-for-jwt",
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}

	token, err := jwtService.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	deps := Dependencies{
		Hints: hints,
	}

	mux := http.NewServeMux()

	authenticated := middleware.Authenticate(
		jwtService,
		&hintUserRepositoryStub{user: user},
	)

	mux.Handle(
		"GET /api/hints",
		authenticated(deps.listHintsHandler()),
	)
	mux.Handle(
		"GET /api/checklist-items/{itemID}/hint",
		authenticated(deps.getHintHandler()),
	)
	mux.Handle(
		"POST /api/hints/{hintID}/read",
		authenticated(deps.readHintHandler()),
	)

	return mux, token
}

func TestListHintsHandler(t *testing.T) {
	user := &model.User{ID: uuid.New()}

	called := false

	hints := &hintReaderStub{
		listFn: func(ctx context.Context, userID uuid.UUID) ([]repository.Hint, error) {
			called = true

			if userID != user.ID {
				t.Fatalf("unexpected user ID: %s", userID)
			}

			return []repository.Hint{}, nil
		},
	}

	handler, token := newHintTestHandler(t, user, hints)

	req := httptest.NewRequest(http.MethodGet, "/api/hints", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if !called {
		t.Fatal("HintReader.List was not called")
	}

	if got := rec.Body.String(); got != "[]\n" {
		t.Fatalf("unexpected response body: %q", got)
	}
}

func TestGetHintHandler_InvalidUUID(t *testing.T) {
	user := &model.User{ID: uuid.New()}

	hints := &hintReaderStub{
		getFn: func(context.Context, uuid.UUID, uuid.UUID) (*repository.Hint, error) {
			t.Fatal("Get must not be called for invalid UUID")
			return nil, nil
		},
	}

	handler, token := newHintTestHandler(t, user, hints)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/checklist-items/not-a-uuid/hint",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	if got := rec.Body.String(); got != "{\"error\":\"invalid item ID\"}\n" {
		t.Fatalf("unexpected response body: %q", got)
	}
}

func TestReadHintHandler(t *testing.T) {
	user := &model.User{ID: uuid.New()}
	hintID := uuid.New()

	called := false

	hints := &hintReaderStub{
		readFn: func(ctx context.Context, userID, receivedHintID uuid.UUID) error {
			called = true

			if userID != user.ID {
				t.Errorf("unexpected user ID: %s", userID)
			}

			if receivedHintID != hintID {
				t.Errorf("unexpected hint ID: %s", receivedHintID)
			}

			return nil
		},
	}

	handler, token := newHintTestHandler(t, user, hints)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/hints/"+hintID.String()+"/read",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	if !called {
		t.Fatal("HintReader.Read was not called")
	}

	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty response body, got %q", rec.Body.String())
	}
}

func TestHintsHandler_Unauthorized(t *testing.T) {
	user := &model.User{ID: uuid.New()}

	hints := &hintReaderStub{
		listFn: func(context.Context, uuid.UUID) ([]repository.Hint, error) {
			t.Fatal("List must not be called without authentication")
			return nil, nil
		},
	}

	handler, _ := newHintTestHandler(t, user, hints)

	req := httptest.NewRequest(http.MethodGet, "/api/hints", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}
