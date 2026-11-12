package handler

import "net/http"

func (d Dependencies) RegisterPersonalRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	for _, method := range []string{"GET", "POST"} {
		mux.Handle(method+" /api/notes",
			authenticated(d.personalHandler(false, true)))

		mux.Handle(method+" /api/calendar/events",
			authenticated(d.personalHandler(true, true)))
	}

	for _, method := range []string{"GET", "PUT", "DELETE"} {
		mux.Handle(method+" /api/notes/{id}",
			authenticated(d.personalHandler(false, false)))

		mux.Handle(method+" /api/calendar/events/{id}",
			authenticated(d.personalHandler(true, false)))
	}
}
