package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func (d Dependencies) RegisterCorporateRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	mux.Handle("GET /api/corporate/me",
		authenticated(d.corporateProfileHandler()))

	mux.Handle("GET /api/corporate/employees/{id}",
		authenticated(d.corporateEmployeeHandler()))

	mux.Handle("GET /api/corporate/tools/{tool}",
		authenticated(d.corporateToolHandler()))
	mux.Handle("GET /api/corporate/tools",
		authenticated(middleware.RequireRole(model.RoleIntern)(d.corporateToolsHandler())))

	mux.Handle("POST /api/corporate/access-requests",
		authenticated(
			middleware.RequireRole(model.RoleIntern)(
				d.createAccessRequestHandler(),
			),
		),
	)

	mux.Handle("GET /api/corporate/access-requests",
		authenticated(d.listAccessRequestsHandler()))

	mux.Handle("GET /api/corporate/access-requests/{id}",
		authenticated(d.getAccessRequestHandler()))
}
