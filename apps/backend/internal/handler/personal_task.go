package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type personalTaskRequest struct {
	// ChecklistType: "professional" (default when omitted) or "common".
	ChecklistType    string  `json:"checklist_type"`
	Position         int     `json:"position"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	ExpectedResult   string  `json:"expected_result"`
	EstimatedMinutes int     `json:"estimated_minutes"`
	RequiresReview   bool    `json:"requires_review"`
	DueAt            *string `json:"due_at"`
}

// internTaskPatchRequest: absent or null fields keep their current value.
type internTaskPatchRequest struct {
	Title            *string `json:"title"`
	Description      *string `json:"description"`
	ExpectedResult   *string `json:"expected_result"`
	EstimatedMinutes *int    `json:"estimated_minutes"`
	DueAt            *string `json:"due_at"`
}

func (d Dependencies) createPersonalTaskHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePathUUID(w, r, "internID", "invalid intern ID")
		if !ok {
			return
		}

		var body personalTaskRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}

		if body.Position < 1 ||
			body.EstimatedMinutes <= 0 ||
			strings.TrimSpace(body.Title) == "" ||
			strings.TrimSpace(body.ExpectedResult) == "" {
			send(w, 400, map[string]string{"error": "invalid task"})
			return
		}
		checklistType := model.ChecklistTypeProfessional
		switch body.ChecklistType {
		case "", string(model.ChecklistTypeProfessional):
		case string(model.ChecklistTypeCommon):
			checklistType = model.ChecklistTypeCommon
		default:
			send(w, 400, map[string]string{"error": "invalid checklist_type"})
			return
		}

		user, _ := middleware.CurrentUser(r.Context())
		dueAt, ok := parseOptionalRFC3339(w, body.DueAt)
		if !ok {
			return
		}

		taskID, err := d.PersonalTasks.CreateInChecklist(
			r.Context(),
			user.ID,
			id,
			checklistType,
			body.Position,
			body.Title,
			body.Description,
			body.ExpectedResult,
			body.EstimatedMinutes,
			body.RequiresReview,
			dueAt,
		)

		switch {
		case errors.Is(err, service.ErrInternNotAssigned):
			send(w, 404, map[string]string{"error": "intern not found"})
		case errors.Is(err, service.ErrInvalidPosition):
			send(w, 400, map[string]string{"error": "invalid position"})
		case err != nil:
			writeServiceError(w, err)
		default:
			send(w, http.StatusCreated, map[string]string{
				"id": taskID.String(),
			})
		}
	}
}

func (d Dependencies) updateInternTaskHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		internID, ok := parsePathUUID(w, r, "internID", "invalid intern ID")
		if !ok {
			return
		}
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		var body internTaskPatchRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}
		dueAt, ok := parseOptionalRFC3339(w, body.DueAt)
		if !ok {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		err := d.ChecklistEditor.UpdateTask(r.Context(), user.ID, internID, itemID, service.TaskEdit{
			Title: body.Title, Description: body.Description, ExpectedResult: body.ExpectedResult,
			EstimatedMinutes: body.EstimatedMinutes, DueAt: dueAt,
		})
		writeInternTaskResult(w, err)
	}
}

func (d Dependencies) removeInternTaskHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		internID, ok := parsePathUUID(w, r, "internID", "invalid intern ID")
		if !ok {
			return
		}
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		writeInternTaskResult(w, d.ChecklistEditor.RemoveTask(r.Context(), user.ID, internID, itemID))
	}
}

func writeInternTaskResult(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, service.ErrInternNotAssigned):
		send(w, http.StatusNotFound, map[string]string{"error": "intern not found"})
	case errors.Is(err, service.ErrInvalidTask):
		send(w, http.StatusBadRequest, map[string]string{"error": "invalid task"})
	default:
		writeServiceError(w, err)
	}
}

// parseOptionalRFC3339 writes 400 and reports false when the value is set but is not RFC 3339.
func parseOptionalRFC3339(w http.ResponseWriter, value *string) (*time.Time, bool) {
	if value == nil {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		send(w, 400, map[string]string{"error": "invalid due_at"})
		return nil, false
	}
	return &parsed, true
}

func (d Dependencies) RegisterPersonalTaskRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	teamleadOnly := middleware.RequireRole(model.RoleTeamlead)
	if d.PersonalTasks != nil {
		mux.Handle(
			"POST /api/teamlead/interns/{internID}/checklist-items",
			authenticated(teamleadOnly(d.createPersonalTaskHandler())),
		)
	}
	if d.ChecklistEditor != nil {
		mux.Handle(
			"PATCH /api/teamlead/interns/{internID}/checklist-items/{itemID}",
			authenticated(teamleadOnly(d.updateInternTaskHandler())),
		)
		mux.Handle(
			"DELETE /api/teamlead/interns/{internID}/checklist-items/{itemID}",
			authenticated(teamleadOnly(d.removeInternTaskHandler())),
		)
	}
}
