package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func (d Dependencies) assignTeamleadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		internID, ok := parsePathUUID(w, r, "internID", "invalid intern ID")
		if !ok {
			return
		}

		var body struct {
			TeamleadID string `json:"teamlead_id"`
		}
		if !decodeJSONBody(w, r, &body) {
			return
		}

		leadID, err := uuid.Parse(body.TeamleadID)
		if err != nil {
			send(w, 400, map[string]string{"error": "invalid teamlead_id"})
			return
		}

		assigner, ok := d.Interns.(interface {
			AssignTeamlead(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
		})
		if !ok {
			send(w, 500, map[string]string{"error": "assignment unavailable"})
			return
		}

		actor, _ := middleware.CurrentUser(r.Context())
		err = assigner.AssignTeamlead(r.Context(), actor.ID, internID, leadID)

		switch {
		case errors.Is(err, service.ErrInternNotAssigned):
			send(w, 404, map[string]string{"error": "intern not found"})
		case errors.Is(err, repository.ErrNotFound):
			send(w, 404, map[string]string{"error": "teamlead not found"})
		case errors.Is(err, service.ErrUserNotTeamlead):
			send(w, 403, map[string]string{"error": "user is not a teamlead"})
		case err != nil:
			send(w, 500, map[string]string{"error": "internal server error"})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
