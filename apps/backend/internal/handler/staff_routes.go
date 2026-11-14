package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func (d Dependencies) RegisterPersonalTaskRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	mux.Handle(
		"POST /api/teamlead/interns/{internID}/checklist-items",
		authenticated(
			middleware.RequireRole(model.RoleTeamlead)(
				d.createPersonalTaskHandler(),
			),
		),
	)
}
