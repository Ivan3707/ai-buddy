package handler

import "net/http"

// RegisterHintRoutes регистрирует HTTP-маршруты подсказок.
// authenticated обеспечивает проверку JWT перед вызовом обработчика.
func (d Dependencies) RegisterHintRoutes(
	mux *http.ServeMux,
	authenticated func(http.Handler) http.Handler,
) {
	mux.Handle(
		"GET /api/hints",
		authenticated(d.listHintsHandler()),
	)

	mux.Handle(
		"GET /api/checklist-items/{itemID}/hint",
		authenticated(d.getHintHandler()),
	)

	mux.Handle(
		"POST /api/hints/{hintID}/read",
		authenticated(d.readHintHandler()),
	)
}
