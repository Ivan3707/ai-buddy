package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type progressSummary struct {
	UserID            string        `json:"user_id"`
	TotalItems        int           `json:"total_items"`
	CompletedItems    int           `json:"completed_items"`
	CompletionPercent float64       `json:"completion_percent"`
	Common            *checklistDTO `json:"common,omitempty"`
	Professional      checklistDTO  `json:"professional"`
}

func (d Dependencies) progressHandler(staff bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := middleware.CurrentUser(r.Context())
		targetID := actor.ID
		if staff {
			var err error
			targetID, err = uuid.Parse(r.PathValue("internID"))
			if err != nil {
				send(w, 400, map[string]string{"error": "invalid intern ID"})
				return
			}
			target, err := d.Users.GetByID(r.Context(), targetID)
			if errors.Is(err, repository.ErrNotFound) {
				send(w, 404, map[string]string{"error": "intern not found"})
				return
			}
			if err != nil {
				send(w, 500, map[string]string{"error": "internal server error"})
				return
			}
			if !service.StaffCanSeeIntern(actor, target) {
				send(w, 404, map[string]string{"error": "intern not found"})
				return
			}
		}
		lists, err := d.Checklists.GetByUserID(r.Context(), targetID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		// Buddy and teamlead see the same data: both checklists and the overall counts.
		total := lists.Common.TotalItems + lists.Professional.TotalItems
		done := lists.Common.CompletedItems + lists.Professional.CompletedItems
		pct := float64(0)
		if total > 0 {
			pct = float64(done) * 100 / float64(total)
		}
		common := toChecklistDTO(lists.Common)
		send(w, 200, progressSummary{UserID: targetID.String(), TotalItems: total, CompletedItems: done, CompletionPercent: pct, Common: &common, Professional: toChecklistDTO(lists.Professional)})
	}
}

func (d Dependencies) RegisterProgressRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	mux.Handle(
		"GET /api/progress",
		authenticated(
			middleware.RequireRole(model.RoleIntern)(
				d.progressHandler(false),
			),
		),
	)
	mux.Handle(
		"GET /api/staff/interns/{internID}/progress",
		authenticated(
			middleware.RequireRole(model.RoleBuddy, model.RoleTeamlead)(
				d.progressHandler(true),
			),
		),
	)
}
