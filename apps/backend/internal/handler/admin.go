package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// RegisterAdminRoutes adds the admin panel API. There is no separate admin role: every route is teamlead only, unless ADMIN_OPEN_FOR_ALL is set (demo).
func RegisterAdminRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, s Admin) {
	h := adminHandler{s: s}
	teamlead := middleware.RequireRole(model.RoleTeamlead)
	gate := func(fn http.HandlerFunc) http.Handler {
		if s.IsOpenForAll() {
			return auth(fn)
		}
		return auth(teamlead(fn))
	}
	route := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, gate(fn)) }
	// every signed-in user may ask whether the panel is open for them (the menu item depends on it)
	mux.Handle("GET /api/admin/access", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		send(w, http.StatusOK, map[string]bool{"allowed": s.IsOpenForAll() || (user != nil && user.Role == model.RoleTeamlead)})
	})))
	route("GET /api/admin/feedback", h.feedback)
	route("GET /api/admin/kb/lint", h.kbLint)
	route("GET /api/admin/kb/gaps", h.kbGaps)
	route("GET /api/admin/accounts", h.accounts)
	route("POST /api/admin/accounts/{userID}/onboarding", h.restartOnboarding)
	route("POST /api/admin/accounts/{userID}/complete-checklists", h.completeChecklists)
	route("GET /api/admin/llm", h.llm)
	route("POST /api/admin/llm/ping", h.llmPing)
	route("GET /api/admin/system", h.system)
	route("POST /api/admin/kb/reload", h.reloadKB)
	route("GET /api/admin/notifications", h.notifications)
	route("POST /api/admin/notifications/stuck_task/check", h.checkStuckTask)
}

type adminHandler struct{ s Admin }

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

// restartOnboarding: the intern sees the welcome tour again at the next login.
func (h adminHandler) restartOnboarding(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "userID", "invalid user ID")
	if !ok {
		return
	}
	switch err := h.s.RestartOnboarding(r.Context(), id.String()); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, sql.ErrNoRows):
		send(w, http.StatusNotFound, map[string]string{"error": "intern not found"})
	default:
		log.Printf("admin restart onboarding: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

// completeChecklists is a DEMO action: all tasks of the intern's checklists become done (100%), so the web app
// offers the checklist feedback form. Idempotent.
func (h adminHandler) completeChecklists(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "userID", "invalid user ID")
	if !ok {
		return
	}
	actor, _ := middleware.CurrentUser(r.Context())
	if actor == nil {
		send(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	switch res, err := h.s.CompleteChecklists(r.Context(), actor.ID, id); {
	case err == nil:
		send(w, http.StatusOK, res)
	case errors.Is(err, repository.ErrNotFound):
		send(w, http.StatusNotFound, map[string]string{"error": "intern not found"})
	default:
		log.Printf("admin complete checklists: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func (h adminHandler) feedback(w http.ResponseWriter, r *http.Request) {
	out, err := h.s.AnswerFeedback(r.Context())
	if err != nil {
		log.Printf("admin feedback: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	send(w, http.StatusOK, out)
}

func (h adminHandler) kbLint(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, 4*time.Minute)
	raw, err := h.s.KBLint(r.Context())
	sendRaw(w, raw, err)
}

func (h adminHandler) kbGaps(w http.ResponseWriter, r *http.Request) {
	extendDeadline(w, 3*time.Minute)
	raw, err := h.s.KBGaps(r.Context())
	sendRaw(w, raw, err)
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
