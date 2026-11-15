package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type checklistEditorStub struct {
	err                 error
	updates, removes    int
	actor, intern, item uuid.UUID
	edit                service.TaskEdit
}

func (s *checklistEditorStub) UpdateTask(_ context.Context, actor, intern, item uuid.UUID, edit service.TaskEdit) error {
	s.updates++
	s.actor, s.intern, s.item, s.edit = actor, intern, item, edit
	return s.err
}

func (s *checklistEditorStub) RemoveTask(_ context.Context, actor, intern, item uuid.UUID) error {
	s.removes++
	s.actor, s.intern, s.item = actor, intern, item
	return s.err
}

func TestInternChecklistEditRoutes(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	stub := &checklistEditorStub{}
	router, token := coverageRouter(t, actor, Dependencies{ChecklistEditor: stub})
	internID, itemID := uuid.New(), uuid.New()
	path := "/api/teamlead/interns/" + internID.String() + "/checklist-items/" + itemID.String()

	coverageRequest(t, router, "", http.MethodPatch, path, `{"title":"x"}`, 401)
	coverageRequest(t, router, token, http.MethodPatch, "/api/teamlead/interns/bad/checklist-items/"+itemID.String(), `{"title":"x"}`, 400)
	coverageRequest(t, router, token, http.MethodPatch, "/api/teamlead/interns/"+internID.String()+"/checklist-items/bad", `{"title":"x"}`, 400)
	coverageRequest(t, router, token, http.MethodPatch, path, `{"title":"x","position":2}`, 400)
	coverageRequest(t, router, token, http.MethodPatch, path, `{"due_at":"tomorrow"}`, 400)
	if stub.updates != 0 {
		t.Fatalf("invalid request reached the service %d times", stub.updates)
	}

	coverageRequest(t, router, token, http.MethodPatch, path, `{"title":"New","estimated_minutes":15,"due_at":"2027-01-02T03:04:05Z"}`, 204)
	if stub.actor != actor.ID || stub.intern != internID || stub.item != itemID ||
		stub.edit.Title == nil || *stub.edit.Title != "New" || stub.edit.EstimatedMinutes == nil || *stub.edit.EstimatedMinutes != 15 ||
		stub.edit.DueAt == nil || !stub.edit.DueAt.Equal(time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)) || stub.edit.Description != nil {
		t.Fatalf("edit passed incorrectly: %+v", stub)
	}
	coverageRequest(t, router, token, http.MethodDelete, path, "", 204)
	if stub.removes != 1 || stub.item != itemID {
		t.Fatalf("remove passed incorrectly: %+v", stub)
	}

	for _, tc := range []struct {
		err    error
		status int
	}{
		{service.ErrInternNotAssigned, 404},
		{repository.ErrNotFound, 404},
		{service.ErrInvalidTask, 400},
		{service.ErrOutdatedProfessionTrack, 409},
		{errors.New("db offline"), 500},
	} {
		stub.err = tc.err
		coverageRequest(t, router, token, http.MethodPatch, path, `{"title":"x"}`, tc.status)
		w := coverageRequest(t, router, token, http.MethodDelete, path, "", tc.status)
		if strings.Contains(w.Body.String(), "db offline") {
			t.Fatal("storage error leaked")
		}
	}

	actor.Role = model.RoleBuddy
	coverageRequest(t, router, token, http.MethodPatch, path, `{"title":"x"}`, 403)
	coverageRequest(t, router, token, http.MethodDelete, path, "", 403)
	actor.Role = model.RoleIntern
	coverageRequest(t, router, token, http.MethodDelete, path, "", 403)
}

type staffSubmissionsStub struct {
	err          error
	actor        *model.User
	intern, item uuid.UUID
	history      []model.ChecklistItemSubmission
}

func (s *staffSubmissionsStub) ListInternSubmissions(_ context.Context, actor *model.User, intern, item uuid.UUID) ([]model.ChecklistItemSubmission, error) {
	s.actor, s.intern, s.item = actor, intern, item
	return s.history, s.err
}

func TestStaffSubmissionHistoryRoute(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	reviewer := uuid.New()
	reviewedAt := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	comment := "Add tests"
	text := "first try"
	stub := &staffSubmissionsStub{history: []model.ChecklistItemSubmission{{
		ID: uuid.New(), ProgressID: uuid.New(), AttemptNumber: 1, ResultText: &text, Status: model.SubmissionRejected,
		SubmittedAt: reviewedAt.Add(-time.Hour), ReviewedAt: &reviewedAt, ReviewedBy: &reviewer, ReviewComment: &comment,
	}}}
	router, token := coverageRouter(t, actor, Dependencies{StaffSubmissions: stub})
	internID, itemID := uuid.New(), uuid.New()
	path := "/api/staff/interns/" + internID.String() + "/checklist-items/" + itemID.String() + "/submissions"

	coverageRequest(t, router, "", http.MethodGet, path, "", 401)
	coverageRequest(t, router, token, http.MethodGet, "/api/staff/interns/bad/checklist-items/"+itemID.String()+"/submissions", "", 400)
	w := coverageRequest(t, router, token, http.MethodGet, path, "", 200)
	if stub.actor != actor || stub.intern != internID || stub.item != itemID {
		t.Fatalf("identity passed incorrectly: %+v", stub)
	}
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("history body: %s %v", w.Body.String(), err)
	}
	if got[0]["status"] != "rejected" || got[0]["review_comment"] != comment || got[0]["reviewed_by"] != reviewer.String() ||
		got[0]["reviewed_at"] != "2026-11-10T09:00:00Z" || got[0]["attempt_number"] != float64(1) {
		t.Fatalf("history entry: %+v", got[0])
	}

	actor.Role = model.RoleBuddy
	coverageRequest(t, router, token, http.MethodGet, path, "", 200)
	stub.err = service.ErrInternNotAssigned
	coverageRequest(t, router, token, http.MethodGet, path, "", 404)
	stub.err = repository.ErrNotFound
	coverageRequest(t, router, token, http.MethodGet, path, "", 404)
	actor.Role = model.RoleIntern
	coverageRequest(t, router, token, http.MethodGet, path, "", 403)
}

func TestTeamleadCannotReviewSubmissions(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	router, token := coverageRouter(t, actor, Dependencies{})
	coverageRequest(t, router, token, http.MethodPost, "/api/buddy/submissions/"+uuid.NewString()+"/review", `{"approved":true}`, 403)
	coverageRequest(t, router, token, http.MethodGet, "/api/buddy/submissions", "", 403)
}

func TestStaffCanSeeIntern(t *testing.T) {
	buddy := &model.User{ID: uuid.New(), Role: model.RoleBuddy}
	lead := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	other := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	intern := &model.User{ID: uuid.New(), Role: model.RoleIntern, BuddyID: &buddy.ID, TeamleadID: &lead.ID}
	notIntern := &model.User{ID: uuid.New(), Role: model.RoleBuddy, BuddyID: &buddy.ID, TeamleadID: &lead.ID}
	for _, tc := range []struct {
		actor, target *model.User
		want          bool
	}{
		{buddy, intern, true}, {lead, intern, true}, {other, intern, false},
		{intern, intern, false}, {buddy, notIntern, false}, {nil, intern, false}, {buddy, nil, false},
	} {
		if got := service.StaffCanSeeIntern(tc.actor, tc.target); got != tc.want {
			t.Fatalf("StaffCanSeeIntern(%v, %v) = %v, want %v", tc.actor, tc.target, got, tc.want)
		}
	}
}

type completeAdminStub struct {
	Admin
	open          bool
	err           error
	calls         int
	actor, intern uuid.UUID
}

func (s *completeAdminStub) IsOpenForAll() bool { return s.open }

func (s *completeAdminStub) CompleteChecklists(_ context.Context, actor, intern uuid.UUID) (service.DemoCompletion, error) {
	s.calls++
	s.actor, s.intern = actor, intern
	return service.DemoCompletion{InternID: intern.String(), ItemsCompleted: 7, SubmissionsApproved: 1}, s.err
}

func TestAdminCompleteChecklistsRoute(t *testing.T) {
	actor := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	stub := &completeAdminStub{}
	router, token := coverageRouter(t, actor, Dependencies{Admin: stub})
	internID := uuid.New()
	path := "/api/admin/accounts/" + internID.String() + "/complete-checklists"

	coverageRequest(t, router, "", http.MethodPost, path, "", 401)
	coverageRequest(t, router, token, http.MethodPost, "/api/admin/accounts/bad/complete-checklists", "", 400)
	w := coverageRequest(t, router, token, http.MethodPost, path, "", 200)
	var got service.DemoCompletion
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.InternID != internID.String() || got.ItemsCompleted != 7 || got.SubmissionsApproved != 1 {
		t.Fatalf("result: %s %v", w.Body.String(), err)
	}
	if stub.actor != actor.ID || stub.intern != internID {
		t.Fatalf("identity passed incorrectly: %+v", stub)
	}
	stub.err = repository.ErrNotFound
	coverageRequest(t, router, token, http.MethodPost, path, "", 404)
	stub.err = errors.New("db offline")
	if w := coverageRequest(t, router, token, http.MethodPost, path, "", 500); strings.Contains(w.Body.String(), "db offline") {
		t.Fatal("storage error leaked")
	}

	// Other roles only with the demo flag ADMIN_OPEN_FOR_ALL=1.
	stub.err = nil
	actor.Role = model.RoleIntern
	calls := stub.calls
	coverageRequest(t, router, token, http.MethodPost, path, "", 403)
	if stub.calls != calls {
		t.Fatal("intern reached the demo action without the demo flag")
	}
	open := &completeAdminStub{open: true}
	router, token = coverageRouter(t, actor, Dependencies{Admin: open})
	coverageRequest(t, router, token, http.MethodPost, "/api/admin/accounts/"+actor.ID.String()+"/complete-checklists", "", 200)
	if open.intern != actor.ID {
		t.Fatalf("demo intern did not complete own checklists: %+v", open)
	}
}
