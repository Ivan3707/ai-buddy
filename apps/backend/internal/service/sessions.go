package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type sessionStore interface {
	Create(context.Context, uuid.UUID, string, time.Time) (uuid.UUID, error)
	Rotate(context.Context, uuid.UUID, string, string, time.Time) (uuid.UUID, error)
	Revoke(context.Context, uuid.UUID, uuid.UUID) error
	IsActive(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

var ErrInvalidRefresh = errors.New("invalid refresh token")

func NewAuthServiceWithSessions(users authUserRepository, jwt *security.JWTService, sessions sessionStore) *AuthService {
	s := NewAuthService(users, jwt)
	s.sessions = sessions
	return s
}
func randomSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func parseRefresh(token string) (uuid.UUID, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(parts[1]) != 43 {
		return uuid.Nil, "", ErrInvalidRefresh
	}
	id, err := uuid.Parse(parts[0])
	if err != nil || id == uuid.Nil {
		return uuid.Nil, "", ErrInvalidRefresh
	}
	return id, parts[1], nil
}
func (s *AuthService) createSession(ctx context.Context, userID uuid.UUID) (string, uuid.UUID, error) {
	secret, err := randomSecret()
	if err != nil {
		return "", uuid.Nil, err
	}
	id, err := s.sessions.Create(ctx, userID, secret, time.Now().Add(30*24*time.Hour))
	if err != nil {
		return "", uuid.Nil, err
	}
	return id.String() + "." + secret, id, nil
}
func (s *AuthService) Refresh(ctx context.Context, token string) (string, string, int64, error) {
	if s.sessions == nil {
		return "", "", 0, ErrInvalidRefresh
	}
	id, secret, err := parseRefresh(token)
	if err != nil {
		return "", "", 0, err
	}
	next, err := randomSecret()
	if err != nil {
		return "", "", 0, err
	}
	userID, err := s.sessions.Rotate(ctx, id, secret, next, time.Now().Add(30*24*time.Hour))
	if errors.Is(err, repository.ErrNotFound) {
		return "", "", 0, ErrInvalidRefresh
	}
	if err != nil {
		return "", "", 0, err
	}
	access, err := s.jwt.GenerateSessionAccessToken(userID, id)
	if err != nil {
		return "", "", 0, err
	}
	return access, id.String() + "." + next, int64(s.jwt.TTL().Seconds()), nil
}
func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID, token string) error {
	if s.sessions == nil {
		return ErrInvalidRefresh
	}
	id, _, err := parseRefresh(token)
	if err != nil {
		return err
	}
	return s.sessions.Revoke(ctx, id, userID)
}
