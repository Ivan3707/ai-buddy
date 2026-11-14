package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

// RegisterAdminRoutes adds the admin panel API. There is no separate admin role: every route is teamlead only.
func RegisterAdminRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, s *service.AdminService) {
	h := adminHandler{s: s}
	teamlead := middleware.RequireRole(model.RoleTeamlead)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, auth(teamlead(fn)))
	}
	route("GET /api/admin/accounts", h.accounts)
	route("GET /api/admin/llm", h.llm)
	route("POST /api/admin/llm/ping", h.llmPing)
	route("GET /api/admin/system", h.system)
	route("POST /api/admin/kb/reload", h.reloadKB)
	route("GET /api/admin/notifications", h.notifications)
	route("POST /api/admin/notifications/stuck_task/check", h.checkStuckTask)
}

type adminHandler struct{ s *service.AdminService }

func extendDeadline(w http.ResponseWriter, d time.Duration) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
}

func (h adminHandler) accounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.s.Accounts(r.Context())
	if err != nil {
		log.Printf("admin accounts: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	send(w, http.StatusOK, map[string]any{"accounts": items})
}

func (h adminHandler) llm(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, time.Minute)
	raw, err := h.s.LLM(r.Context())
	sendRaw(w, raw, err)
}

func (h adminHandler) llmPing(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, time.Minute)
	raw, err := h.s.LLMPing(r.Context())
	sendRaw(w, raw, err)
}

func (h adminHandler) reloadKB(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, 4*time.Minute)
	raw, err := h.s.ReloadKB(r.Context())
	sendRaw(w, raw, err)
}

func (h adminHandler) system(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, time.Minute)
	send(w, http.StatusOK, h.s.System(r.Context()))
}

func (h adminHandler) notifications(w http.ResponseWriter, r *http.Request) {
	send(w, http.StatusOK, map[string]any{"items": h.s.Notifications(r.Context())})
}

func (h adminHandler) checkStuckTask(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, 3*time.Minute)
	res, err := h.s.CheckStuckTask(r.Context())
	if err != nil {
		log.Printf("admin stuck_task check: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	send(w, http.StatusOK, res)
}
