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
	if deps.PersonalTasks != nil {
		Dependencies{
			PersonalTasks: deps.PersonalTasks,
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
		Checklists:  deps.Checklists,
		Items:       deps.Items,
		Submissions: deps.Submissions,
	}.RegisterChecklistRoutes(mux, authenticated)
	if deps.Feedback != nil {
		Dependencies{Feedback: deps.Feedback}.RegisterFeedbackRoutes(mux, authenticated)
	}

	mux.Handle("POST /api/chat", authenticated(chatHandler(aiURL)))
	if deps.Assistant != nil {
		RegisterAssistantRoutes(mux, authenticated, deps.Assistant)
	}
	if deps.Admin != nil {
		RegisterAdminRoutes(mux, authenticated, deps.Admin)
	}

	return middleware.InstrumentHTTP(mux)
}
