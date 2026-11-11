package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

const authIntegrationPassword = "correct-password-123"

func createAuthIntegrationUser(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	role model.UserRole,
) *model.User {
	t.Helper()
	passwordHash, err := security.HashPassword(authIntegrationPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{
		Email:        "auth-" + uuid.NewString() + "@example.com",
		PasswordHash: passwordHash,
		Role:         role,
	}
	if role == model.RoleIntern {
		profession := model.ProfessionBackend
		user.Profession = &profession
	}
	if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatalf("create auth user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			user.ID,
		); err != nil {
			t.Errorf("cleanup auth user: %v", err)
		}
	})
	return user
}

func newAuthIntegrationService(
	t *testing.T,
	db *sql.DB,
) (*AuthService, *security.JWTService) {
	t.Helper()
	jwtService, err := security.NewJWTService(
		"test-secret-that-is-at-least-32-bytes-long",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}
	return NewAuthService(repository.NewUserRepository(db), jwtService), jwtService
}

func TestAuthService_FirstAndSubsequentLoginIntegration(t *testing.T) {
	for _, role := range []model.UserRole{model.RoleIntern, model.RoleBuddy} {
		t.Run(string(role), func(t *testing.T) {
			db, ctx := openChecklistTestDB(t)
			user := createAuthIntegrationUser(t, db, ctx, role)
			svc, jwtService := newAuthIntegrationService(t, db)

			first, err := svc.Login(ctx, user.Email, authIntegrationPassword)
			if err != nil {
				t.Fatalf("first login: %v", err)
			}
			if !first.IsFirstLogin {
				t.Fatal("expected first login flag")
			}
			if _, err := jwtService.ValidateAccessToken(first.AccessToken); err != nil {
				t.Fatalf("validate first JWT: %v", err)
			}

			second, err := svc.Login(ctx, user.Email, authIntegrationPassword)
			if err != nil {
				t.Fatalf("second login: %v", err)
			}
			if second.IsFirstLogin {
				t.Fatal("expected subsequent login flag")
			}
			if _, err := jwtService.ValidateAccessToken(second.AccessToken); err != nil {
				t.Fatalf("validate second JWT: %v", err)
			}

			stored, err := repository.NewUserRepository(db).GetByID(ctx, user.ID)
			if err != nil {
				t.Fatalf("get user after login: %v", err)
			}
			if stored.LastLoginAt == nil {
				t.Fatal("expected last_login_at to be updated")
			}
		})
	}
}

func TestAuthService_InvalidCredentialsDoNotRecordLoginIntegration(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	user := createAuthIntegrationUser(t, db, ctx, model.RoleIntern)
	svc, _ := newAuthIntegrationService(t, db)

	if _, err := svc.Login(ctx, user.Email, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: expected ErrInvalidCredentials, got %v", err)
	}
	if _, err := svc.Login(ctx, "missing-"+uuid.NewString()+"@example.com", authIntegrationPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("missing user: expected ErrInvalidCredentials, got %v", err)
	}

	stored, err := repository.NewUserRepository(db).GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user after failed logins: %v", err)
	}
	if stored.LastLoginAt != nil {
		t.Fatalf("failed login changed last_login_at: %v", stored.LastLoginAt)
	}
}

func TestAuthService_ConcurrentFirstLoginIntegration(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	user := createAuthIntegrationUser(t, db, ctx, model.RoleIntern)
	svc, jwtService := newAuthIntegrationService(t, db)

	start := make(chan struct{})
	type loginOutcome struct {
		result *LoginResult
		err    error
	}
	outcomes := make(chan loginOutcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			result, err := svc.Login(ctx, user.Email, authIntegrationPassword)
			outcomes <- loginOutcome{result: result, err: err}
		}()
	}
	close(start)

	firstLoginCount := 0
	for i := 0; i < 2; i++ {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("concurrent login: %v", outcome.err)
		}
		if outcome.result == nil {
			t.Fatal("concurrent login returned nil result")
		}
		if outcome.result.IsFirstLogin {
			firstLoginCount++
		}
		tokenUserID, err := jwtService.ValidateAccessToken(outcome.result.AccessToken)
		if err != nil {
			t.Fatalf("validate concurrent JWT: %v", err)
		}
		if tokenUserID != user.ID {
			t.Fatalf("unexpected JWT user: got %s want %s", tokenUserID, user.ID)
		}
	}
	if firstLoginCount != 1 {
		t.Fatalf("expected exactly one first login, got %d", firstLoginCount)
	}
}
