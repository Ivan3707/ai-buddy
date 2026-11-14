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
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type stubFeedback struct {
	created bool
	user    uuid.UUID
}

func (s *stubFeedback) Get(context.Context, uuid.UUID, uuid.UUID) (*service.Feedback, error) {
	return nil, nil
}
func (s *stubFeedback) Create(_ context.Context, user, checklist uuid.UUID, rating int, comment string) (*service.Feedback, error) {
	s.created = true
	s.user = user
	return &service.Feedback{ID: uuid.New(), UserID: user, ChecklistID: checklist, Rating: rating, Comment: comment, DeliveryStatus: "pending"}, nil
}
func TestFeedbackRoutesIdentityAndRole(t *testing.T) {
	jwt, err := security.NewJWTService(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	checklist := uuid.New()
	for _, role := range []model.UserRole{model.RoleIntern, model.RoleBuddy, model.RoleTeamlead} {
		actor := &model.User{ID: uuid.New(), Email: "actor@example.test", Role: role}
		token, err := jwt.GenerateAccessToken(actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		feedback := &stubFeedback{}
		router := handler.NewRouter("", handler.Dependencies{Users: &testUserRepository{user: actor}, JWT: jwt, Feedback: feedback})
		request := httptest.NewRequest(http.MethodPost, "/api/checklists/"+checklist.String()+"/feedback", strings.NewReader(`{"rating":5,"comment":"ok"}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if role == model.RoleIntern {
			if response.Code != 201 || !feedback.created || feedback.user != actor.ID {
				t.Fatalf("intern: %d %+v", response.Code, feedback)
			}
		} else if response.Code != 403 || feedback.created {
			t.Fatalf("staff role %s: %d", role, response.Code)
		}
	}
}
