package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func (d Dependencies) RegisterChecklistRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	internOnly := middleware.RequireRole(model.RoleIntern)
	buddyOnly := middleware.RequireRole(model.RoleBuddy)

	mux.Handle(
		"GET /api/checklists",
		authenticated(internOnly(d.checklistsHandler())),
	)
	mux.Handle(
		"POST /api/checklist-items/{itemID}/complete",
		authenticated(internOnly(d.completeItemHandler())),
	)
	mux.Handle(
		"POST /api/checklist-items/{itemID}/submissions",
		authenticated(internOnly(d.submitItemHandler())),
	)
	mux.Handle(
		"GET /api/checklist-items/{itemID}/submissions",
		authenticated(internOnly(d.submissionHistoryHandler())),
	)
	mux.Handle(
		"GET /api/buddy/submissions",
		authenticated(buddyOnly(d.pendingSubmissionsHandler())),
	)
	// Only the buddy reviews: a teamlead gets 403 here (and ProgressService.ReviewSubmission rejects non-buddies too).
	mux.Handle(
		"POST /api/buddy/submissions/{submissionID}/review",
		authenticated(buddyOnly(d.reviewSubmissionHandler())),
	)
	if d.StaffSubmissions != nil {
		mux.Handle(
			"GET /api/staff/interns/{internID}/checklist-items/{itemID}/submissions",
			authenticated(middleware.RequireRole(model.RoleBuddy, model.RoleTeamlead)(d.staffSubmissionHistoryHandler())),
		)
	}
}
