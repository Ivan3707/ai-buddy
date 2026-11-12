package main

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
)

func newHandler(
	aiURL string,
	onboarding []byte,
	deps handlerDependencies,
) http.Handler {
	return handler.NewRouter(aiURL, onboarding, handler.Dependencies{
		Auth:          deps.auth,
		Users:         deps.users,
		JWT:           deps.jwt,
		Interns:       deps.interns,
		Checklists:    deps.checklists,
		Items:         deps.items,
		Submissions:   deps.submissions,
		PersonalTasks: deps.personalTasks,
		Hints:         deps.hints,
		Personal:      deps.personal,
		Corporate:     deps.corporate,
		SessionCheck:  deps.sessionCheck,
		Sessions:      deps.sessions,
	})
}
