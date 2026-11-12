package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
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
			if target.Role != model.RoleIntern || (actor.Role == model.RoleBuddy && (target.BuddyID == nil || *target.BuddyID != actor.ID)) || (actor.Role == model.RoleTeamlead && (target.TeamleadID == nil || *target.TeamleadID != actor.ID)) {
				send(w, 404, map[string]string{"error": "intern not found"})
				return
			}
		}
		lists, err := d.Checklists.GetByUserID(r.Context(), targetID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		total := lists.Common.TotalItems + lists.Professional.TotalItems
		done := lists.Common.CompletedItems + lists.Professional.CompletedItems
		if staff && actor.Role == model.RoleTeamlead {
			total = lists.Professional.TotalItems
			done = lists.Professional.CompletedItems
		}
		pct := float64(0)
		if total > 0 {
			pct = float64(done) * 100 / float64(total)
		}
		common := toChecklistDTO(lists.Common)
		out := progressSummary{UserID: targetID.String(), TotalItems: total, CompletedItems: done, CompletionPercent: pct, Common: &common, Professional: toChecklistDTO(lists.Professional)}
		if staff && actor.Role == model.RoleTeamlead {
			out.Common = nil
		}
		send(w, 200, out)
	}
}
