package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

const testJWTSecret = "test-secret-that-is-at-least-32-bytes-long"
const testPassword = "correct-password-123"

func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type testUserRepository struct {
	user            *model.User
	getByEmailErr   error
	getByEmailCalls int
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
	r.getByEmailCalls++
	if r.getByEmailErr != nil {
		return nil, r.getByEmailErr
	}
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
