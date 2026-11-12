package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type personalTaskRequest struct {
	Position         int    `json:"position"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	ExpectedResult   string `json:"expected_result"`
	EstimatedMinutes int    `json:"estimated_minutes"`
	RequiresReview   bool   `json:"requires_review"`
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

		user, _ := middleware.CurrentUser(r.Context())

		taskID, err := d.PersonalTasks.Create(
			r.Context(),
			user.ID,
			id,
			body.Position,
			body.Title,
			body.Description,
			body.ExpectedResult,
			body.EstimatedMinutes,
			body.RequiresReview,
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
