package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-that-is-at-least-32-bytes-long"

func TestJWTGenerateAndValidate(t *testing.T) {
	service, err := NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	userID := uuid.New()

	token, err := service.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	gotID, err := service.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if gotID != userID {
		t.Fatalf("got user ID %s, want %s", gotID, userID)
	}
}

func TestJWTRejectsTamperedToken(t *testing.T) {
	service, err := NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}

	userID := uuid.New()

	token, err := service.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// Изменяем первый символ подписи.
	replacement := "A"
	if parts[2][0] == 'A' {
		replacement = "B"
	}

	parts[2] = replacement + parts[2][1:]
	tamperedToken := strings.Join(parts, ".")

	_, err = service.ValidateAccessToken(tamperedToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTRejectsExpiredToken(t *testing.T) {
	service, err := NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Subject:   uuid.New().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	expiredToken, err := token.SignedString(service.secret)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.ValidateAccessToken(expiredToken); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestNewJWTServiceRejectsInvalidConfig(t *testing.T) {
	if _, err := NewJWTService("short", 15*time.Minute); err == nil {
		t.Fatal("expected short secret to be rejected")
	}

	if _, err := NewJWTService(testJWTSecret, 0); err == nil {
		t.Fatal("expected zero TTL to be rejected")
	}
}
