package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func corporateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrCorporateUnavailable):
		send(w, 503, map[string]string{"error": "corporate service unavailable"})
	case errors.Is(err, service.ErrCorporateNotFound):
		send(w, 404, map[string]string{"error": "not found"})
	case errors.Is(err, service.ErrCorporateForbidden):
		send(w, 403, map[string]string{"error": "forbidden"})
	case errors.Is(err, service.ErrIdempotencyConflict):
		send(w, 409, map[string]string{"error": "idempotency conflict"})
	case errors.Is(err, service.ErrInvalidCorporateRequest):
		send(w, 400, map[string]string{"error": "invalid request"})
	default:
		send(w, 500, map[string]string{"error": "internal server error"})
	}
}
func (d Dependencies) corporateProfileHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		profile, err := d.Corporate.Profile(user)
		if err != nil {
			corporateError(w, err)
			return
		}
		send(w, 200, profile)
	}
}
func (d Dependencies) corporateEmployeeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := middleware.CurrentUser(r.Context())
		self, err := d.Corporate.Profile(actor)
		if err != nil {
			corporateError(w, err)
			return
		}
		target, ok := d.Corporate.Directory.ByID(r.PathValue("id"))
		if !ok {
			send(w, 404, map[string]string{"error": "not found"})
			return
		}
		if target.ID != self.ID {
			if target.Role != model.RoleIntern || ((actor.Role != model.RoleBuddy || target.BuddyID != self.ID) && (actor.Role != model.RoleTeamlead || target.ManagerID != self.ID)) {
				send(w, 404, map[string]string{"error": "not found"})
				return
			}
			// Local assignment is authoritative for application access.
			local, err := d.corporateUserByEmail(r, target.Email)
			if err != nil || (actor.Role == model.RoleBuddy && (local.BuddyID == nil || *local.BuddyID != actor.ID)) || (actor.Role == model.RoleTeamlead && (local.TeamleadID == nil || *local.TeamleadID != actor.ID)) {
				send(w, 404, map[string]string{"error": "not found"})
				return
			}
			verified, err := d.Corporate.Profile(local)
			if err != nil || verified.ID != target.ID {
				send(w, 404, map[string]string{"error": "not found"})
				return
			}
		}
		send(w, 200, target)
	}
}
func (d Dependencies) corporateUserByEmail(r *http.Request, email string) (*model.User, error) {
	reader, ok := d.Users.(interface {
		GetByEmail(context.Context, string) (*model.User, error)
	})
	if !ok {
		return nil, service.ErrCorporateUnavailable
	}
	return reader.GetByEmail(r.Context(), email)
}

type accessRequestBody struct {
	Tool           string `json:"tool"`
	IdempotencyKey string `json:"idempotency_key"`
	Confirm        bool   `json:"confirm"`
}

func (d Dependencies) createAccessRequestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body accessRequestBody
		if !decodeJSONBody(w, r, &body) {
			return
		}
		if !body.Confirm {
			send(w, 400, map[string]string{"error": "confirmation required"})
			return
		}
		body.Tool = strings.TrimSpace(body.Tool)
		if body.Tool == "" || len(body.Tool) > 80 || len(body.IdempotencyKey) < 8 || len(body.IdempotencyKey) > 128 {
			send(w, 400, map[string]string{"error": "invalid request"})
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		result, created, err := d.Corporate.CreateRequest(r.Context(), user, body.Tool, body.IdempotencyKey)
		if err != nil {
			corporateError(w, err)
			return
		}
		if created {
			send(w, 201, result)
		} else {
			send(w, 200, result)
		}
	}
}
func (d Dependencies) getAccessRequestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			send(w, 400, map[string]string{"error": "invalid request ID"})
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		result, err := d.Corporate.GetRequest(r.Context(), user, id)
		if err != nil {
			corporateError(w, err)
			return
		}
		send(w, 200, result)
	}
}
func (d Dependencies) listAccessRequestsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		result, err := d.Corporate.ListRequests(r.Context(), user)
		if err != nil {
			corporateError(w, err)
			return
		}
		send(w, 200, result)
	}
}
func (d Dependencies) corporateToolHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		result, err := d.Corporate.ToolInfo(user, r.PathValue("tool"))
		if err != nil {
			corporateError(w, err)
			return
		}
		send(w, 200, result)
	}
}

func (d Dependencies) corporateToolsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		result, err := d.Corporate.ToolsForUser(user)
		if err != nil {
			corporateError(w, err)
			return
		}
		send(w, http.StatusOK, result)
	}
}

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
