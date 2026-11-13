package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type testTeamReader struct {
	interns []model.User
	leads   []model.User
}

func (r *testTeamReader) ListAssignedInterns(_ context.Context, _ uuid.UUID, _ model.UserRole) ([]model.User, error) {
	return r.interns, nil
}

func (r *testTeamReader) ListTeamleads(_ context.Context) ([]model.User, error) {
	return r.leads, nil
}
func (r *testTeamReader) ListBuddies(_ context.Context) ([]model.User, error) {
	return r.leads, nil
}

func TestTeamRoutesReturnPublicAssignedUsers(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Email: "buddy@example.com", Role: model.RoleBuddy, CreatedAt: time.Now()}
	first, last := "Ирина", "Стажёрова"
	team := &testTeamReader{
		interns: []model.User{{ID: uuid.New(), Email: "intern@example.com", PasswordHash: "secret", Role: model.RoleIntern, FirstName: &first, LastName: &last, CreatedAt: time.Now()}},
		leads:   []model.User{{ID: uuid.New(), Email: "lead@example.com", PasswordHash: "secret", Role: model.RoleTeamlead, CreatedAt: time.Now()}},
	}
	jwt, err := security.NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.GenerateAccessToken(actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := handler.NewRouter("", []byte(demo), handler.Dependencies{Users: &testUserRepository{user: actor}, JWT: jwt, Team: team})

	for _, path := range []string{"/api/staff/interns", "/api/staff/teamleads"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authorizedRequest(http.MethodGet, path, "", token))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: got %d: %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "password") {
			t.Fatalf("%s exposed a password field: %s", path, w.Body.String())
		}
	}
}

func TestTeamleadsRouteRejectsTeamlead(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Email: "lead@example.com", Role: model.RoleTeamlead, CreatedAt: time.Now()}
	jwt, _ := security.NewJWTService(testJWTSecret, 15*time.Minute)
	token, _ := jwt.GenerateAccessToken(actor.ID)
	router := handler.NewRouter("", []byte(demo), handler.Dependencies{Users: &testUserRepository{user: actor}, JWT: jwt, Team: &testTeamReader{}})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/staff/teamleads", "", token))
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", w.Code)
	}
}

func TestBuddiesRouteForTeamlead(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Email: "lead@example.com", Role: model.RoleTeamlead, CreatedAt: time.Now()}
	jwt, _ := security.NewJWTService(testJWTSecret, 15*time.Minute)
	token, _ := jwt.GenerateAccessToken(actor.ID)
	router := handler.NewRouter("", []byte(demo), handler.Dependencies{Users: &testUserRepository{user: actor}, JWT: jwt, Team: &testTeamReader{leads: []model.User{{ID: uuid.New(), Email: "buddy@example.com", Role: model.RoleBuddy, CreatedAt: time.Now()}}}})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/staff/buddies", "", token))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "buddy@example.com") {
		t.Fatalf("unexpected buddy list: %d %s", w.Code, w.Body.String())
	}
}
