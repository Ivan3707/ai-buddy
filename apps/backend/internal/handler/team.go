package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func (d Dependencies) RegisterTeamRoutes(mux *http.ServeMux, authenticated func(http.Handler) http.Handler) {
	mux.Handle("GET /api/staff/interns", authenticated(middleware.RequireRole(model.RoleBuddy, model.RoleTeamlead)(d.assignedInternsHandler())))
	mux.Handle("GET /api/staff/teamleads", authenticated(middleware.RequireRole(model.RoleBuddy)(d.teamleadsHandler())))
	mux.Handle("GET /api/staff/buddies", authenticated(middleware.RequireRole(model.RoleTeamlead)(d.buddiesHandler())))
}

func (d Dependencies) buddiesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := d.Team.ListBuddies(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result := make([]publicUser, 0, len(users))
		for i := range users {
			result = append(result, toPublicUser(&users[i]))
		}
		send(w, http.StatusOK, result)
	}
}

func (d Dependencies) assignedInternsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := middleware.CurrentUser(r.Context())
		users, err := d.Team.ListAssignedInterns(r.Context(), actor.ID, actor.Role)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result := make([]publicUser, 0, len(users))
		for i := range users {
			result = append(result, toPublicUser(&users[i]))
		}
		send(w, http.StatusOK, result)
	}
}

func (d Dependencies) teamleadsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := d.Team.ListTeamleads(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result := make([]publicUser, 0, len(users))
		for i := range users {
			result = append(result, toPublicUser(&users[i]))
		}
		send(w, http.StatusOK, result)
	}
}
