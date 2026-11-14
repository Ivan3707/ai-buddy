package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type failingSessionStore struct {
	rotateErr   error
	revokeErr   error
	rotateCalls int
	revokeCalls int
}

func (*failingSessionStore) Create(context.Context, uuid.UUID, string, time.Time) (uuid.UUID, error) {
	return uuid.Nil, errors.New("unexpected Create")
}
func (s *failingSessionStore) Rotate(context.Context, uuid.UUID, string, string, time.Time) (uuid.UUID, error) {
	s.rotateCalls++
	return uuid.Nil, s.rotateErr
}
func (s *failingSessionStore) Revoke(context.Context, uuid.UUID, uuid.UUID, string) error {
	s.revokeCalls++
	return s.revokeErr
}
func (*failingSessionStore) IsActive(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, errors.New("unexpected IsActive")
}

func TestSessionFailuresNeverIssueTokens(t *testing.T) {
	jwt, err := security.NewJWTService("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	storageFailure := errors.New("session store unavailable")
	valid := uuid.NewString() + "." + strings.Repeat("a", 43)
	userID := uuid.New()
	for _, tc := range []struct {
		name     string
		token    string
		storeErr error
		want     error
		calls    int
	}{
		{"malformed", "bad", nil, ErrInvalidRefresh, 0},
		{"missing session", valid, repository.ErrNotFound, ErrInvalidRefresh, 1},
		{"storage error", valid, storageFailure, storageFailure, 1},
	} {
		t.Run("refresh "+tc.name, func(t *testing.T) {
			store := &failingSessionStore{rotateErr: tc.storeErr}
			svc := NewAuthServiceWithSessions(nil, jwt, store)
			access, refresh, ttl, err := svc.Refresh(context.Background(), tc.token)
			if !errors.Is(err, tc.want) || access != "" || refresh != "" || ttl != 0 || store.rotateCalls != tc.calls {
				t.Fatalf("refresh returned access=%q refresh=%q ttl=%d err=%v calls=%d", access, refresh, ttl, err, store.rotateCalls)
			}
		})
		t.Run("logout "+tc.name, func(t *testing.T) {
			store := &failingSessionStore{revokeErr: tc.storeErr}
			svc := NewAuthServiceWithSessions(nil, jwt, store)
			err := svc.Logout(context.Background(), userID, tc.token)
			if !errors.Is(err, tc.want) || store.revokeCalls != tc.calls {
				t.Fatalf("logout err=%v calls=%d", err, store.revokeCalls)
			}
		})
	}
	t.Run("sessions unavailable", func(t *testing.T) {
		svc := NewAuthService(nil, jwt)
		access, refresh, ttl, err := svc.Refresh(context.Background(), valid)
		if !errors.Is(err, ErrInvalidRefresh) || access != "" || refresh != "" || ttl != 0 {
			t.Fatalf("refresh: %q %q %d %v", access, refresh, ttl, err)
		}
		if err := svc.Logout(context.Background(), userID, valid); !errors.Is(err, ErrInvalidRefresh) {
			t.Fatalf("logout: %v", err)
		}
	})
}
