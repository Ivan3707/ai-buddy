package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type mockAuthUserRepository struct {
	user             *model.User
	err              error
	recordErr        error
	isFirstLogin     bool
	recordLoginCalls int
}

func (m *mockAuthUserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*model.User, error) {
	return m.user, m.err
}

func (m *mockAuthUserRepository) RecordLogin(
	ctx context.Context,
	id uuid.UUID,
) (bool, error) {
	m.recordLoginCalls++
	if m.recordErr != nil {
		return false, m.recordErr
	}
	return m.isFirstLogin, nil
}

func newTestAuthService(
	t *testing.T,
	user *model.User,
	repoErr error,
) *AuthService {
	t.Helper()

	jwtService, err := security.NewJWTService(
		"test-secret-that-is-at-least-32-bytes-long",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}

	return NewAuthService(
		&mockAuthUserRepository{
			user:         user,
			err:          repoErr,
			isFirstLogin: true,
		},
		jwtService,
	)
}

func TestAuthServiceLogin(t *testing.T) {
	password := "correct-password-123"

	passwordHash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &model.User{
		ID:           uuid.New(),
		Email:        "intern@example.com",
		PasswordHash: passwordHash,
		Role:         model.RoleIntern,
	}

	t.Run("success", func(t *testing.T) {
		svc := newTestAuthService(t, user, nil)

		result, err := svc.Login(
			context.Background(),
			" intern@example.com ",
			password,
		)
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		if result.AccessToken == "" {
			t.Fatal("expected access token")
		}

		if result.ExpiresIn != 900 {
			t.Fatalf("expected 900 seconds, got %d", result.ExpiresIn)
		}

		if result.User.ID != user.ID {
			t.Fatalf("expected user %s, got %s", user.ID, result.User.ID)
		}
		if !result.IsFirstLogin {
			t.Fatal("expected first login")
		}

		jwtService, err := security.NewJWTService(
			"test-secret-that-is-at-least-32-bytes-long",
			15*time.Minute,
		)
		if err != nil {
			t.Fatalf("create JWT service: %v", err)
		}

		tokenUserID, err := jwtService.ValidateAccessToken(result.AccessToken)
		if err != nil {
			t.Fatalf("validate access token: %v", err)
		}

		if tokenUserID != user.ID {
			t.Fatalf("expected token subject %s, got %s", user.ID, tokenUserID)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		svc := newTestAuthService(t, user, nil)

		_, err := svc.Login(
			context.Background(),
			user.Email,
			"wrong-password",
		)

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		svc := newTestAuthService(t, nil, repository.ErrNotFound)

		_, err := svc.Login(
			context.Background(),
			"unknown@example.com",
			password,
		)

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("database unavailable")
		svc := newTestAuthService(t, nil, repoErr)

		_, err := svc.Login(
			context.Background(),
			"intern@example.com",
			password,
		)

		if err == nil {
			t.Fatal("expected repository error")
		}
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected wrapped repository error, got %v", err)
		}
	})

	t.Run("record login error", func(t *testing.T) {
		recordErr := errors.New("write failed")
		repo := &mockAuthUserRepository{user: user, recordErr: recordErr}
		jwtService, err := security.NewJWTService(
			"test-secret-that-is-at-least-32-bytes-long",
			15*time.Minute,
		)
		if err != nil {
			t.Fatalf("create JWT service: %v", err)
		}
		result, err := NewAuthService(repo, jwtService).Login(
			context.Background(),
			user.Email,
			password,
		)
		if result != nil || !errors.Is(err, recordErr) {
			t.Fatalf("expected record error without login result, got result=%v err=%v", result, err)
		}
	})
}
