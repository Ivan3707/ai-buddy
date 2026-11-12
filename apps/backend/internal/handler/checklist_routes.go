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
	mux.Handle(
		"POST /api/buddy/submissions/{submissionID}/review",
		authenticated(buddyOnly(d.reviewSubmissionHandler())),
	)
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
