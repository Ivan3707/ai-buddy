package security

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSessionJWTContainsSessionID(t *testing.T) {
	jwt, err := NewJWTService("12345678901234567890123456789012", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	userID, sessionID := uuid.New(), uuid.New()
	token, err := jwt.GenerateSessionAccessToken(userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	gotUser, gotSession, err := jwt.ValidateSessionAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser != userID || gotSession != sessionID {
		t.Fatalf("got %s %s", gotUser, gotSession)
	}
}
