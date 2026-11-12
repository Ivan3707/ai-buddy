package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type checklistReaderStub struct {
	result *service.UserChecklists
	err    error
}

func (s *checklistReaderStub) GetByUserID(context.Context, uuid.UUID) (*service.UserChecklists, error) {
	return s.result, s.err
}

type checklistItemServiceStub struct {
	completeErr error
	submit      model.ChecklistItemSubmission
	submitErr   error
	history     []model.ChecklistItemSubmission
	historyErr  error
	userID      uuid.UUID
	itemID      uuid.UUID
	resultText  *string
	resultURL   *string
}

func (s *checklistItemServiceStub) CompleteItem(_ context.Context, userID, itemID uuid.UUID) error {
	s.userID, s.itemID = userID, itemID
	return s.completeErr
}

func (s *checklistItemServiceStub) SubmitItem(
	_ context.Context,
	userID, itemID uuid.UUID,
	resultText, resultURL *string,
) (model.ChecklistItemSubmission, error) {
	s.userID, s.itemID, s.resultText, s.resultURL = userID, itemID, resultText, resultURL
	return s.submit, s.submitErr
}

func (s *checklistItemServiceStub) ListSubmissions(
	_ context.Context,
	userID, itemID uuid.UUID,
) ([]model.ChecklistItemSubmission, error) {
	s.userID, s.itemID = userID, itemID
	return s.history, s.historyErr
}

type buddySubmissionServiceStub struct {
	pending      []service.PendingSubmission
	pendingErr   error
	reviewErr    error
	buddyID      uuid.UUID
	submissionID uuid.UUID
	approved     bool
	comment      *string
	reviewCalled bool
}

func (s *buddySubmissionServiceStub) ListPendingSubmissions(
	_ context.Context,
	buddyID uuid.UUID,
) ([]service.PendingSubmission, error) {
	s.buddyID = buddyID
	return s.pending, s.pendingErr
}

func (s *buddySubmissionServiceStub) ReviewSubmission(
	_ context.Context,
	buddyID, submissionID uuid.UUID,
	approved bool,
	comment *string,
) error {
	s.reviewCalled = true
	s.buddyID, s.submissionID, s.approved, s.comment = buddyID, submissionID, approved, comment
	return s.reviewErr
}

func newChecklistAPIHandler(
	t *testing.T,
	role model.UserRole,
	checklists handler.ChecklistReader,
	items handler.ChecklistItemService,
	submissions handler.BuddySubmissionService,
) (http.Handler, string, *model.User) {
	t.Helper()
	user := &model.User{
		ID:        uuid.New(),
		Email:     "api-user@example.com",
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	users := &testUserRepository{user: user}
	jwtService, err := security.NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("create JWT service: %v", err)
	}
	token, err := jwtService.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	handler := handler.NewRouter("", []byte(demo), handler.Dependencies{
		Auth:        service.NewAuthService(users, jwtService),
		Users:       users,
		JWT:         jwtService,
		Checklists:  checklists,
		Items:       items,
		Submissions: submissions,
	})
	return handler, token, user
}

func TestChecklistAPI_GetChecklists(t *testing.T) {
	estimated := 45
	itemID, stageID, commonID, professionalID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	result := &service.UserChecklists{
		Common: service.ChecklistWithProgress{
			Checklist: model.Checklist{ID: commonID, Name: "Common", Type: model.ChecklistTypeCommon},
			Stages: []service.ChecklistStageWithProgress{{
				Stage: model.ChecklistStage{ID: stageID, Title: "Week 1", Position: 1},
				Items: []service.ChecklistItemProgress{{
					Item:   model.ChecklistItem{ID: itemID, Title: "Task", Position: 1, RequiresReview: true, EstimatedMinutes: &estimated},
					Status: model.StatusActive,
				}},
			}},
			TotalItems: 1,
		},
		Professional: service.ChecklistWithProgress{
			Checklist: model.Checklist{ID: professionalID, Name: "Backend", Type: model.ChecklistTypeProfessional},
			Stages:    []service.ChecklistStageWithProgress{},
		},
	}
	handler, token, _ := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{result: result}, &checklistItemServiceStub{}, &buddySubmissionServiceStub{})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/checklists", "", token))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["common"] == nil || response["professional"] == nil || !strings.Contains(w.Body.String(), `"estimated_minutes":45`) {
		t.Fatalf("checklist response is incomplete: %s", w.Body.String())
	}
}

func TestChecklistAPI_CompleteItem(t *testing.T) {
	itemID := uuid.New()
	items := &checklistItemServiceStub{}
	handler, token, user := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/checklist-items/"+itemID.String()+"/complete", "", token))
	if w.Code != http.StatusNoContent || items.userID != user.ID || items.itemID != itemID {
		t.Fatalf("unexpected completion result: status=%d user=%s item=%s", w.Code, items.userID, items.itemID)
	}

	items.completeErr = service.ErrSubmissionRequired
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/checklist-items/"+itemID.String()+"/complete", "", token))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "requires a submission") {
		t.Fatalf("review item completion: got %d %s", w.Code, w.Body.String())
	}
}

func TestChecklistAPI_SubmitItem(t *testing.T) {
	itemID, submissionID, progressID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name string
		body string
	}{
		{name: "text", body: `{"result_text":"done"}`},
		{name: "url", body: `{"result_url":"https://example.com/pr/1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := &checklistItemServiceStub{submit: model.ChecklistItemSubmission{
				ID: submissionID, ProgressID: progressID, AttemptNumber: 1,
				Status: model.SubmissionPending, SubmittedAt: now,
			}}
			handler, token, user := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/checklist-items/"+itemID.String()+"/submissions", tt.body, token))
			if w.Code != http.StatusCreated {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if items.userID != user.ID || items.itemID != itemID || !strings.Contains(w.Body.String(), submissionID.String()) {
				t.Fatalf("submission request/response mismatch: %s", w.Body.String())
			}
		})
	}
}

func TestChecklistAPI_SubmitItemValidation(t *testing.T) {
	itemID := uuid.New()
	items := &checklistItemServiceStub{submitErr: service.ErrInvalidSubmission}
	handler, token, _ := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})
	tests := []struct {
		name   string
		target string
		body   string
	}{
		{name: "empty", target: "/api/checklist-items/" + itemID.String() + "/submissions", body: `{}`},
		{name: "invalid uuid", target: "/api/checklist-items/not-a-uuid/submissions", body: `{"result_text":"done"}`},
		{name: "invalid json", target: "/api/checklist-items/" + itemID.String() + "/submissions", body: `broken`},
		{name: "unknown field", target: "/api/checklist-items/" + itemID.String() + "/submissions", body: `{"result_text":"done","status":"approved"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, authorizedRequest(http.MethodPost, tt.target, tt.body, token))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChecklistAPI_SubmissionHistory(t *testing.T) {
	itemID, submissionID, progressID := uuid.New(), uuid.New(), uuid.New()
	comment := "revise"
	reviewedAt := time.Now().UTC()
	items := &checklistItemServiceStub{history: []model.ChecklistItemSubmission{{
		ID: submissionID, ProgressID: progressID, AttemptNumber: 1,
		Status: model.SubmissionRejected, SubmittedAt: reviewedAt.Add(-time.Hour),
		ReviewedAt: &reviewedAt, ReviewComment: &comment,
	}}}
	handler, token, _ := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/checklist-items/"+itemID.String()+"/submissions", "", token))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"review_comment":"revise"`) {
		t.Fatalf("history: got %d %s", w.Code, w.Body.String())
	}

	items.historyErr = repository.ErrNotFound
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/checklist-items/"+uuid.NewString()+"/submissions", "", token))
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign history: got %d %s", w.Code, w.Body.String())
	}
}

func TestChecklistAPI_BuddyQueue(t *testing.T) {
	submissionID, progressID, internID, itemID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	text := "result"
	buddy := &buddySubmissionServiceStub{pending: []service.PendingSubmission{{
		Submission: model.ChecklistItemSubmission{
			ID: submissionID, ProgressID: progressID, AttemptNumber: 2,
			ResultText: &text, Status: model.SubmissionPending, SubmittedAt: time.Now().UTC(),
		},
		InternID: internID, ItemID: itemID, ItemTitle: "Task",
	}}}
	handler, token, user := newChecklistAPIHandler(t, model.RoleBuddy, &checklistReaderStub{}, &checklistItemServiceStub{}, buddy)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/buddy/submissions", "", token))
	if w.Code != http.StatusOK || buddy.buddyID != user.ID || !strings.Contains(w.Body.String(), internID.String()) {
		t.Fatalf("buddy queue: got %d %s", w.Code, w.Body.String())
	}
}

func TestChecklistAPI_ReviewSubmission(t *testing.T) {
	submissionID := uuid.New()
	tests := []struct {
		name       string
		body       string
		serviceErr error
		wantStatus int
		called     bool
	}{
		{name: "approve", body: `{"approved":true}`, wantStatus: http.StatusNoContent, called: true},
		{name: "reject", body: `{"approved":false,"comment":"add tests"}`, wantStatus: http.StatusNoContent, called: true},
		{name: "reject without comment", body: `{"approved":false}`, wantStatus: http.StatusBadRequest},
		{name: "repeat review", body: `{"approved":true}`, serviceErr: service.ErrSubmissionAlreadyReviewed, wantStatus: http.StatusConflict, called: true},
		{name: "foreign buddy", body: `{"approved":true}`, serviceErr: service.ErrInternNotAssigned, wantStatus: http.StatusNotFound, called: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buddy := &buddySubmissionServiceStub{reviewErr: tt.serviceErr}
			handler, token, user := newChecklistAPIHandler(t, model.RoleBuddy, &checklistReaderStub{}, &checklistItemServiceStub{}, buddy)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/buddy/submissions/"+submissionID.String()+"/review", tt.body, token))
			if w.Code != tt.wantStatus || buddy.reviewCalled != tt.called {
				t.Fatalf("got status=%d called=%v body=%s", w.Code, buddy.reviewCalled, w.Body.String())
			}
			if tt.called && (buddy.buddyID != user.ID || buddy.submissionID != submissionID) {
				t.Fatal("review did not use JWT buddy or path submission ID")
			}
		})
	}
}

func TestChecklistAPI_AuthenticationAndRoles(t *testing.T) {
	items := &checklistItemServiceStub{}
	internHandler, internToken, _ := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{result: &service.UserChecklists{}}, items, &buddySubmissionServiceStub{})
	buddyHandler, buddyToken, _ := newChecklistAPIHandler(t, model.RoleBuddy, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})

	tests := []struct {
		name    string
		handler http.Handler
		target  string
		token   string
		status  int
	}{
		{name: "missing JWT", handler: internHandler, target: "/api/checklists", status: http.StatusUnauthorized},
		{name: "invalid JWT", handler: internHandler, target: "/api/checklists", token: "invalid", status: http.StatusUnauthorized},
		{name: "intern buddy endpoint", handler: internHandler, target: "/api/buddy/submissions", token: internToken, status: http.StatusForbidden},
		{name: "buddy intern endpoint", handler: buddyHandler, target: "/api/checklists", token: buddyToken, status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			tt.handler.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Fatalf("got %d want %d: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestChecklistAPI_InternalErrorIsNotLeaked(t *testing.T) {
	items := &checklistItemServiceStub{historyErr: errors.New("ERROR: relation secret_table does not exist SQLSTATE 42P01")}
	handler, token, _ := newChecklistAPIHandler(t, model.RoleIntern, &checklistReaderStub{}, items, &buddySubmissionServiceStub{})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodGet, "/api/checklist-items/"+uuid.NewString()+"/submissions", "", token))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret_table") || !strings.Contains(w.Body.String(), "internal server error") {
		t.Fatalf("unsafe internal error response: %d %s", w.Code, w.Body.String())
	}
}

func TestChecklistAPI_LegacyReviewRouteRemoved(t *testing.T) {
	handler, token, _ := newChecklistAPIHandler(t, model.RoleBuddy, &checklistReaderStub{}, &checklistItemServiceStub{}, &buddySubmissionServiceStub{})
	target := "/api/interns/" + uuid.NewString() + "/checklist-items/" + uuid.NewString() + "/review"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodPost, target, `{"approved":true}`, token))
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy route must be removed, got %d", w.Code)
	}
}
