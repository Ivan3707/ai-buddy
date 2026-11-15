package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
)

func NewRouter(
	aiURL string,
	deps Dependencies,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		send(w, http.StatusOK, map[string]string{
			"status": "ok",
			"mode":   "mock",
		})
	})

	// /health — liveness (процесс жив). /ready — readiness: БД подготовлена (миграции + каталог), его использует Compose.
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if deps.Ready != nil {
			if err := deps.Ready(r.Context()); err != nil {
				send(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": err.Error()})
				return
			}
		}
		send(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	authenticated := func(handler http.Handler) http.Handler {
		if deps.SessionCheck != nil {
			return middleware.Authenticate(deps.JWT, deps.Users, deps.SessionCheck)(handler)
		}
		return middleware.Authenticate(deps.JWT, deps.Users)(handler)
	}
	Dependencies{
		Auth:     deps.Auth,
		Users:    deps.Users,
		Sessions: deps.Sessions,
	}.RegisterAuthRoutes(mux, authenticated)

	if deps.Corporate != nil {
		Dependencies{
			Corporate: deps.Corporate,
			Users:     deps.Users,
		}.RegisterCorporateRoutes(mux, authenticated)
	}
	if deps.Checklists != nil {
		Dependencies{
			Users:      deps.Users,
			Checklists: deps.Checklists,
		}.RegisterProgressRoutes(mux, authenticated)
	}
	if deps.PersonalTasks != nil || deps.ChecklistEditor != nil {
		Dependencies{
			PersonalTasks:   deps.PersonalTasks,
			ChecklistEditor: deps.ChecklistEditor,
		}.RegisterPersonalTaskRoutes(mux, authenticated)
	}
	if deps.Hints != nil {
		hintDeps := Dependencies{
			Hints: deps.Hints,
		}

		hintDeps.RegisterHintRoutes(mux, authenticated)
	}
	if deps.Personal != nil {
		Dependencies{
			Personal: deps.Personal,
		}.RegisterPersonalRoutes(mux, authenticated)
	}

	if deps.Team != nil {
		Dependencies{Team: deps.Team}.RegisterTeamRoutes(mux, authenticated)
	}
	Dependencies{
		Checklists:       deps.Checklists,
		Items:            deps.Items,
		Submissions:      deps.Submissions,
		StaffSubmissions: deps.StaffSubmissions,
	}.RegisterChecklistRoutes(mux, authenticated)
	if deps.Feedback != nil {
		Dependencies{Feedback: deps.Feedback}.RegisterFeedbackRoutes(mux, authenticated)
	}

	mux.Handle("POST /api/chat", authenticated(chatHandler(aiURL)))
	if deps.Assistant != nil {
		RegisterAssistantRoutes(mux, authenticated, deps.Assistant)
	}
	if deps.TaskComments != nil {
		RegisterTaskCommentRoutes(mux, authenticated, deps.TaskComments)
	}
	if deps.Admin != nil {
		RegisterAdminRoutes(mux, authenticated, deps.Admin)
	}

	return middleware.InstrumentHTTP(mux)
}
