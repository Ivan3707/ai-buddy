package handler

import (
	"errors"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
	"net/http"
)

func (d Dependencies) personalHandler(event bool, collection bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		s := d.Personal
		if collection {
			if r.Method == http.MethodGet {
				if event {
					v, e := s.ListEvents(r.Context(), user.ID)
					if e != nil {
						writeServiceError(w, e)
					} else {
						send(w, 200, v)
					}
				} else {
					v, e := s.ListNotes(r.Context(), user.ID)
					if e != nil {
						writeServiceError(w, e)
					} else {
						send(w, 200, v)
					}
				}
				return
			}
			if event {
				var v repository.CalendarEvent
				if !decodeJSONBody(w, r, &v) {
					return
				}
				if e := s.CreateEvent(r.Context(), user.ID, &v); e != nil {
					personalError(w, e)
				} else {
					send(w, 201, v)
				}
			} else {
				var v repository.Note
				if !decodeJSONBody(w, r, &v) {
					return
				}
				if e := s.CreateNote(r.Context(), user.ID, &v); e != nil {
					personalError(w, e)
				} else {
					send(w, 201, v)
				}
			}
			return
		}
		id, ok := parsePathUUID(w, r, "id", "invalid ID")
		if !ok {
			return
		}
		if event {
			switch r.Method {
			case http.MethodGet:
				v, e := s.GetEvent(r.Context(), user.ID, id)
				if e != nil {
					personalError(w, e)
				} else {
					send(w, 200, v)
				}
			case http.MethodPut:
				var v repository.CalendarEvent
				if !decodeJSONBody(w, r, &v) {
					return
				}
				v.ID = id
				if e := s.UpdateEvent(r.Context(), user.ID, &v); e != nil {
					personalError(w, e)
				} else {
					send(w, 200, v)
				}
			case http.MethodDelete:
				if e := s.DeleteEvent(r.Context(), user.ID, id); e != nil {
					personalError(w, e)
				} else {
					w.WriteHeader(204)
				}
			}
		} else {
			switch r.Method {
			case http.MethodGet:
				v, e := s.GetNote(r.Context(), user.ID, id)
				if e != nil {
					personalError(w, e)
				} else {
					send(w, 200, v)
				}
			case http.MethodPut:
				var v repository.Note
				if !decodeJSONBody(w, r, &v) {
					return
				}
				v.ID = id
				if e := s.UpdateNote(r.Context(), user.ID, &v); e != nil {
					personalError(w, e)
				} else {
					send(w, 200, v)
				}
			case http.MethodDelete:
				if e := s.DeleteNote(r.Context(), user.ID, id); e != nil {
					personalError(w, e)
				} else {
					w.WriteHeader(204)
				}
			}
		}
	}
}
func personalError(w http.ResponseWriter, e error) {
	if errors.Is(e, service.ErrInvalidPersonal) {
		send(w, 400, map[string]string{"error": e.Error()})
	} else {
		writeServiceError(w, e)
	}
}
