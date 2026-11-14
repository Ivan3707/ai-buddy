package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

func TestProtectedRoutesRejectInvalidAuthorization(t *testing.T) {
	router, valid, user := newTestHandler(t, "")
	otherSecret, err := security.NewJWTService("another-secret-that-is-at-least-32-bytes-long", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wrongSignature, err := otherSecret.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	missingUser, err := security.NewJWTService(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	unknownToken, err := missingUser.GenerateAccessToken(uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.RegisteredClaims{
		Issuer: "ai-buddy", Subject: user.ID.String(),
		IssuedAt:  jwtlib.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(-time.Hour)),
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, authorization string
	}{
		{"absent", ""},
		{"no bearer scheme", valid},
		{"wrong scheme", "Basic " + valid},
		{"extra bearer field", "Bearer " + valid + " extra"},
		{"malformed JWT", "Bearer invalid.jwt.value"},
		{"wrong signature", "Bearer " + wrongSignature},
		{"expired JWT", "Bearer " + expired},
		{"nonexistent user", "Bearer " + unknownToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range []struct{ method, path string }{{http.MethodGet, "/api/auth/me"}, {http.MethodPost, "/api/chat"}} {
				r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"message":"private"}`))
				if tc.authorization != "" {
					r.Header.Set("Authorization", tc.authorization)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != 401 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
					t.Fatalf("%s %s: status=%d content-type=%q", route.method, route.path, w.Code, w.Header().Get("Content-Type"))
				}
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body) != 1 || body["error"] != "unauthorized" {
					t.Fatalf("unexpected refusal body: %q err=%v", w.Body.String(), err)
				}
				if strings.Contains(w.Body.String(), user.Email) || strings.Contains(w.Body.String(), user.ID.String()) || strings.Contains(w.Body.String(), "private") {
					t.Fatal("refusal exposed user or request data")
				}
			}
		})
	}
}
