package handler

import (
	"net/http"
)

func (d Dependencies) RegisterAuthRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	mux.HandleFunc(
		"POST /api/auth/login",
		d.loginHandler(),
	)

	mux.HandleFunc(
		"POST /api/auth/refresh",
		d.refreshHandler(),
	)

	mux.Handle(
		"GET /api/auth/me",
		authenticated(d.meHandler()),
	)

	mux.Handle(
		"POST /api/auth/logout",
		authenticated(d.logoutHandler()),
	)

}
