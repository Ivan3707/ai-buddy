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
	Revoke(context.Context, uuid.UUID, uuid.UUID, string) error
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
// sessionVerifier is implemented by the Postgres store; fakes without it simply cannot link sessions.
type sessionVerifier interface {
	Verify(context.Context, uuid.UUID, string) (uuid.UUID, error)
}

// Link opens a NEW session of the same user for another client (token exchange). The caller proves possession of
// its own refresh token, which is not rotated. Every client keeps its own refresh-token chain: one shared chain
// would be revoked by replay detection as soon as two clients refresh at the same time.
func (s *AuthService) Link(ctx context.Context, token string) (string, string, int64, error) {
	verifier, ok := s.sessions.(sessionVerifier)
	if !ok || s.sessions == nil {
		return "", "", 0, ErrInvalidRefresh
	}
	id, secret, err := parseRefresh(token)
	if err != nil {
		return "", "", 0, err
	}
	userID, err := verifier.Verify(ctx, id, secret)
	if errors.Is(err, repository.ErrNotFound) {
		return "", "", 0, ErrInvalidRefresh
	}
	if err != nil {
		return "", "", 0, err
	}
	refresh, sessionID, err := s.createSession(ctx, userID)
	if err != nil {
		return "", "", 0, err
	}
	access, err := s.jwt.GenerateSessionAccessToken(userID, sessionID)
	if err != nil {
		return "", "", 0, err
	}
	return access, refresh, int64(s.jwt.TTL().Seconds()), nil
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID, token string) error {
	if s.sessions == nil {
		return ErrInvalidRefresh
	}
	id, secret, err := parseRefresh(token)
	if err != nil {
		return err
	}
	if err := s.sessions.Revoke(ctx, id, userID, secret); errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidRefresh
	} else {
		return err
	}
}
