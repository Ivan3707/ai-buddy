package handler

import (
	"net/http"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
)

// listHintsHandler возвращает подсказки текущего пользователя.
func (d Dependencies) listHintsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())

		hints, err := d.Hints.List(r.Context(), user.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		send(w, http.StatusOK, hints)
	}
}

// getHintHandler возвращает подсказку для конкретного задания.
func (d Dependencies) getHintHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		itemID, ok := parsePathUUID(
			w,
			r,
			"itemID",
			"invalid item ID",
		)
		if !ok {
			return
		}

		user, _ := middleware.CurrentUser(r.Context())

		hint, err := d.Hints.Get(r.Context(), user.ID, itemID)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		send(w, http.StatusOK, hint)
	}
}

// readHintHandler отмечает подсказку как прочитанную.
func (d Dependencies) readHintHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hintID, ok := parsePathUUID(
			w,
			r,
			"hintID",
			"invalid hint ID",
		)
		if !ok {
			return
		}

		user, _ := middleware.CurrentUser(r.Context())

		if err := d.Hints.Read(r.Context(), user.ID, hintID); err != nil {
			writeServiceError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
