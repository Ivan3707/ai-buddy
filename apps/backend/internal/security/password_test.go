package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVerifyPasswordContextCanceledWhileSlotsBusy(t *testing.T) {
	hash, err := HashPassword("correct-password-123")
	if err != nil {
		t.Fatal(err)
	}
	for range maxConcurrentArgon {
		argonSlots <- struct{}{}
	}
	defer func() {
		for range maxConcurrentArgon {
			<-argonSlots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	valid, err := VerifyPasswordContext(ctx, "correct-password-123", hash)
	if valid || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled check, got valid=%v err=%v", valid, err)
	}
}

func TestHashPassword(t *testing.T) {
	password := "correct-password-123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}

	valid, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !valid {
		t.Fatal("expected password to be valid")
	}
}

func TestHashPasswordUsesUniqueSalt(t *testing.T) {
	password := "same-password-123"

	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("expected different hashes because salts should be unique")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password-123")
	if err != nil {
		t.Fatal(err)
	}

	valid, err := VerifyPassword("wrong-password-123", hash)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("expected wrong password to be rejected")
	}
}

func TestVerifyPasswordRejectsInvalidHash(t *testing.T) {
	_, err := VerifyPassword("password", "invalid-hash")
	if err == nil {
		t.Fatal("expected invalid hash error")
	}
}
