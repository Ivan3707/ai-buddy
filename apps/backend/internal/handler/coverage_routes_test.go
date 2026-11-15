package handler

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
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func coverageRouter(t *testing.T, user *model.User, deps Dependencies) (http.Handler, string) {
	t.Helper()
	jwt, err := security.NewJWTService("test-secret-at-least-32-bytes-long-for-jwt", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	deps.JWT = jwt
	deps.Users = &hintUserRepositoryStub{user: user}
	token, err := jwt.GenerateAccessToken(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter("", deps), token
}

func coverageRequest(t *testing.T, router http.Handler, token, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, want, w.Body.String())
	}
	return w
}

type feedbackReadStub struct {
	wantUser, wantChecklist uuid.UUID
	value                   *service.Feedback
	err                     error
	calls                   int
}

func (s *feedbackReadStub) Get(_ context.Context, user, checklist uuid.UUID) (*service.Feedback, error) {
	s.calls++
	if user != s.wantUser || checklist != s.wantChecklist {
		return nil, errors.New("wrong feedback identity")
	}
	return s.value, s.err
}
func (*feedbackReadStub) Create(context.Context, uuid.UUID, uuid.UUID, int, string) (*service.Feedback, error) {
	return nil, errors.New("unexpected create")
}

func TestFeedbackGetHTTPContract(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleIntern}
	id := uuid.New()
	stub := &feedbackReadStub{wantUser: actor.ID, wantChecklist: id}
	router, token := coverageRouter(t, actor, Dependencies{Feedback: stub})
	path := "/api/checklists/" + id.String() + "/feedback"
	coverageRequest(t, router, "", http.MethodGet, path, "", 401)
	coverageRequest(t, router, token, http.MethodGet, "/api/checklists/bad/feedback", "", 400)
	if stub.calls != 0 {
		t.Fatal("unauthorized or invalid request reached feedback store")
	}
	w := coverageRequest(t, router, token, http.MethodGet, path, "", 200)
	var empty map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &empty); err != nil || string(empty["feedback"]) != "null" {
		t.Fatalf("empty feedback: %s, err=%v", w.Body.String(), err)
	}
	stub.value = &service.Feedback{ID: uuid.New(), UserID: actor.ID, ChecklistID: id, Rating: 5, Comment: "mine"}
	w = coverageRequest(t, router, token, http.MethodGet, path, "", 200)
	var got struct {
		Feedback service.Feedback `json:"feedback"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Feedback.ID != stub.value.ID || got.Feedback.UserID != actor.ID {
		t.Fatalf("feedback: %s, err=%v", w.Body.String(), err)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{service.ErrFeedbackNotFound, 404}, {service.ErrFeedbackIncomplete, 409},
		{service.ErrFeedbackInvalid, 400}, {errors.New("db offline"), 500},
	} {
		stub.err, stub.value = tc.err, nil
		w = coverageRequest(t, router, token, http.MethodGet, path, "", tc.status)
		if strings.Contains(w.Body.String(), "db offline") {
			t.Fatal("storage error leaked over HTTP")
		}
	}
	if stub.calls != 6 {
		t.Fatalf("feedback Get calls=%d", stub.calls)
	}
}

func TestCorporateToolsHTTPPermissionsAndDirectoryErrors(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Email: "intern@example.test", Role: model.RoleIntern}
	directory, err := service.NewCorporateDirectory([]byte(`[
		{"id":"i","email":"intern@example.test","role":"intern","allowed_tools":["vpn"]},
		{"id":"j","email":"other@example.test","role":"intern","allowed_tools":["wiki"]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	corporate := service.NewCorporateService(nil, directory)
	router, token := coverageRouter(t, actor, Dependencies{Corporate: corporate})
	path := "/api/corporate/tools"
	coverageRequest(t, router, "", http.MethodGet, path, "", 401)
	w := coverageRequest(t, router, token, http.MethodGet, path, "", 200)
	var tools []service.CorporateTool
	if err := json.Unmarshal(w.Body.Bytes(), &tools); err != nil || len(tools) != 2 || tools[0].Tool != "vpn" || !tools[0].Allowed || tools[1].Tool != "wiki" || tools[1].Allowed {
		t.Fatalf("tools visibility/order: %s err=%v", w.Body.String(), err)
	}
	corporate.Unavailable = true
	coverageRequest(t, router, token, http.MethodGet, path, "", 503)
	corporate.Unavailable = false
	actor.Email = "absent@example.test"
	coverageRequest(t, router, token, http.MethodGet, path, "", 404)
	actor.Email = "intern@example.test"
	actor.Role = model.RoleBuddy
	coverageRequest(t, router, token, http.MethodGet, path, "", 403)
}

type personalCreateStub struct {
	err           error
	calls         int
	due           *time.Time
	actor, intern uuid.UUID
	checklistType model.ChecklistType
}

func (s *personalCreateStub) CreateInChecklist(_ context.Context, actor, intern uuid.UUID, checklistType model.ChecklistType, _ int, _, _, _ string, _ int, _ bool, due *time.Time) (uuid.UUID, error) {
	s.calls++
	s.actor, s.intern, s.checklistType = actor, intern, checklistType
	s.due = due
	return uuid.New(), s.err
}

func TestCreatePersonalTaskHTTPValidationAndServiceErrors(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	stub := &personalCreateStub{}
	router, token := coverageRouter(t, actor, Dependencies{PersonalTasks: stub})
	id := uuid.New()
	path := "/api/teamlead/interns/" + id.String() + "/checklist-items"
	valid := `{"position":1,"title":"Read guide","expected_result":"Summary","estimated_minutes":20}`
	coverageRequest(t, router, "", http.MethodPost, path, valid, 401)
	coverageRequest(t, router, token, http.MethodPost, "/api/teamlead/interns/bad/checklist-items", valid, 400)
	for _, bad := range []string{`{"position":0,"title":"T","expected_result":"R","estimated_minutes":1}`, `{"position":1,"title":" ","expected_result":"R","estimated_minutes":1}`, `{"position":1,"title":"T","expected_result":"R","estimated_minutes":0}`, `{"position":1,"title":"T","expected_result":"R","estimated_minutes":1,"due_at":"tomorrow"}`, `{"position":1,"title":"T","expected_result":"R","estimated_minutes":1,"extra":1}`} {
		coverageRequest(t, router, token, http.MethodPost, path, bad, 400)
	}
	if stub.calls != 0 {
		t.Fatalf("invalid request reached service %d times", stub.calls)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{service.ErrInternNotAssigned, 404}, {service.ErrInvalidPosition, 400}, {errors.New("db offline"), 500}} {
		stub.err = tc.err
		w := coverageRequest(t, router, token, http.MethodPost, path, valid, tc.status)
		if strings.Contains(w.Body.String(), "db offline") {
			t.Fatal("storage error leaked")
		}
	}
	stub.err = nil
	w := coverageRequest(t, router, token, http.MethodPost, path, strings.TrimSuffix(valid, "}")+`,"due_at":"2027-01-02T03:04:05Z"}`, 201)
	if stub.actor != actor.ID || stub.intern != id || stub.due == nil || stub.due.UTC().Format(time.RFC3339) != "2027-01-02T03:04:05Z" ||
		stub.checklistType != model.ChecklistTypeProfessional {
		t.Fatalf("identity/due/default checklist mismatch: %+v", stub)
	}
	coverageRequest(t, router, token, http.MethodPost, path, strings.TrimSuffix(valid, "}")+`,"checklist_type":"common"}`, 201)
	if stub.checklistType != model.ChecklistTypeCommon {
		t.Fatalf("common checklist not passed: %s", stub.checklistType)
	}
	calls := stub.calls
	coverageRequest(t, router, token, http.MethodPost, path, strings.TrimSuffix(valid, "}")+`,"checklist_type":"other"}`, 400)
	if stub.calls != calls {
		t.Fatal("invalid checklist_type reached the service")
	}
	var result struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.ID == uuid.Nil {
		t.Fatalf("created id: %s err=%v", w.Body.String(), err)
	}
}

func TestHintRoutesRegistrationAndOwnershipContract(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleIntern}
	itemID, hintID := uuid.New(), uuid.New()
	hints := &hintReaderStub{
		listFn: func(_ context.Context, user uuid.UUID) ([]repository.Hint, error) {
			if user != actor.ID {
				t.Fatal("wrong list owner")
			}
			return []repository.Hint{}, nil
		},
		getFn: func(_ context.Context, user, item uuid.UUID) (*repository.Hint, error) {
			if user != actor.ID || item != itemID {
				t.Fatal("wrong get owner/item")
			}
			return nil, repository.ErrNotFound
		},
		readFn: func(_ context.Context, user, hint uuid.UUID) error {
			if user != actor.ID || hint != hintID {
				t.Fatal("wrong read owner/hint")
			}
			return repository.ErrNotFound
		},
	}
	router, token := coverageRouter(t, actor, Dependencies{Hints: hints})
	coverageRequest(t, router, "", http.MethodGet, "/api/hints", "", 401)
	coverageRequest(t, router, token, http.MethodGet, "/api/hints", "", 200)
	coverageRequest(t, router, token, http.MethodGet, "/api/checklist-items/bad/hint", "", 400)
	coverageRequest(t, router, token, http.MethodGet, "/api/checklist-items/"+itemID.String()+"/hint", "", 404)
	coverageRequest(t, router, token, http.MethodPost, "/api/hints/bad/read", "", 400)
	coverageRequest(t, router, token, http.MethodPost, "/api/hints/"+hintID.String()+"/read", "", 404)
}
