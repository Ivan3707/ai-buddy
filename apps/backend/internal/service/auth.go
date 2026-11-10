package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

// AuthService использует только чтение пользователей.
// Создание пользователей остаётся ответственностью UserService.
type authUserRepository interface {
	GetByEmail(ctx context.Context, email string) (*model.User, error)
}

type AuthService struct {
	users authUserRepository
	jwt   *security.JWTService
}

type LoginResult struct {
	AccessToken string
	ExpiresIn   int64
	User        *model.User
}

func NewAuthService(
	users authUserRepository,
	jwtService *security.JWTService,
) *AuthService {
	return &AuthService{
		users: users,
		jwt:   jwtService,
	}
}

func (s *AuthService) Login(
	ctx context.Context,
	email string,
	password string,
) (*LoginResult, error) {
	email = strings.TrimSpace(email)

	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	if user == nil {
		return nil, errors.New("user repository returned nil user")
	}

	passwordValid, err := security.VerifyPassword(
		password,
		user.PasswordHash,
	)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !passwordValid {
		return nil, ErrInvalidCredentials
	}

	token, err := s.jwt.GenerateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	return &LoginResult{
		AccessToken: token,
		ExpiresIn:   int64(s.jwt.TTL().Seconds()),
		User:        user,
	}, nil
}
