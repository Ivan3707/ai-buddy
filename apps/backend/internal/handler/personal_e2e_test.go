package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestPersonalHTTPUserIsolation(t *testing.T) {
	db, ctx := openP0TestDB(t)
	first := createP0User(t, db, ctx, model.RoleIntern)
	second := createP0User(t, db, ctx, model.RoleIntern)
	jwt, err := security.NewJWTService(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	router := handler.NewRouter("", handler.Dependencies{
		Users: repository.NewUserRepository(db), JWT: jwt,
		Personal: service.NewPersonalService(repository.NewPersonalRepository(db)),
	})
	firstToken, secondToken := p0Token(t, jwt, first), p0Token(t, jwt, second)

	t.Run("notes", func(t *testing.T) {
		path := "/api/notes"
		var own, foreign repository.Note
		p0Call(t, router, http.MethodPost, path, firstToken, map[string]string{"title": "First private", "content": "one"}, 201, &own)
		p0Call(t, router, http.MethodPost, path, secondToken, map[string]string{"title": "Second private", "content": "two"}, 201, &foreign)
		if own.ID == uuid.Nil || foreign.ID == uuid.Nil || own.ID == foreign.ID {
			t.Fatal("invalid created note IDs")
		}
		var firstList, secondList []repository.Note
		p0Call(t, router, http.MethodGet, path, firstToken, nil, 200, &firstList)
		p0Call(t, router, http.MethodGet, path, secondToken, nil, 200, &secondList)
		if len(firstList) != 1 || firstList[0].ID != own.ID || len(secondList) != 1 || secondList[0].ID != foreign.ID {
			t.Fatalf("note lists leaked or lost records: first=%+v second=%+v", firstList, secondList)
		}
		ownPath := path + "/" + own.ID.String()
		var loaded repository.Note
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 200, &loaded)
		if loaded.ID != own.ID || loaded.Content != "one" {
			t.Fatalf("own note read: %+v", loaded)
		}
		p0Call(t, router, http.MethodGet, ownPath, secondToken, nil, 404, nil)
		p0Call(t, router, http.MethodPut, ownPath, secondToken, map[string]string{"title": "Stolen", "content": "changed"}, 404, nil)
		p0Call(t, router, http.MethodDelete, ownPath, secondToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 200, &loaded)
		if loaded.Content != "one" {
			t.Fatalf("foreign mutation changed note: %+v", loaded)
		}
		p0Call(t, router, http.MethodPut, ownPath, firstToken, map[string]string{"title": "Updated", "content": "new"}, 200, &loaded)
		if loaded.Title != "Updated" || loaded.Content != "new" {
			t.Fatalf("own update failed: %+v", loaded)
		}
		p0Call(t, router, http.MethodDelete, ownPath, firstToken, nil, 204, nil)
		p0Call(t, router, http.MethodDelete, ownPath, firstToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, path+"/"+foreign.ID.String(), secondToken, nil, 200, &loaded)
		if loaded.ID != foreign.ID || loaded.Content != "two" {
			t.Fatalf("second user's note changed: %+v", loaded)
		}
	})

	t.Run("calendar events", func(t *testing.T) {
		path := "/api/calendar/events"
		start := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
		newEvent := func(title string) map[string]any {
			return map[string]any{"title": title, "description": "private", "starts_at": start, "ends_at": start.Add(time.Hour)}
		}
		var own, foreign repository.CalendarEvent
		p0Call(t, router, http.MethodPost, path, firstToken, newEvent("First event"), 201, &own)
		p0Call(t, router, http.MethodPost, path, secondToken, newEvent("Second event"), 201, &foreign)
		if own.ID == uuid.Nil || foreign.ID == uuid.Nil || own.ID == foreign.ID {
			t.Fatal("invalid created event IDs")
		}
		var firstList, secondList []repository.CalendarEvent
		p0Call(t, router, http.MethodGet, path, firstToken, nil, 200, &firstList)
		p0Call(t, router, http.MethodGet, path, secondToken, nil, 200, &secondList)
		if len(firstList) != 1 || firstList[0].ID != own.ID || len(secondList) != 1 || secondList[0].ID != foreign.ID {
			t.Fatalf("event lists leaked or lost records: first=%+v second=%+v", firstList, secondList)
		}
		ownPath := path + "/" + own.ID.String()
		var loaded repository.CalendarEvent
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 200, &loaded)
		if loaded.ID != own.ID || loaded.Title != "First event" {
			t.Fatalf("own event read: %+v", loaded)
		}
		p0Call(t, router, http.MethodGet, ownPath, secondToken, nil, 404, nil)
		p0Call(t, router, http.MethodPut, ownPath, secondToken, newEvent("Stolen"), 404, nil)
		p0Call(t, router, http.MethodDelete, ownPath, secondToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 200, &loaded)
		if loaded.Title != "First event" {
			t.Fatalf("foreign mutation changed event: %+v", loaded)
		}
		p0Call(t, router, http.MethodPut, ownPath, firstToken, newEvent("Updated event"), 200, &loaded)
		if loaded.Title != "Updated event" {
			t.Fatalf("own update failed: %+v", loaded)
		}
		p0Call(t, router, http.MethodDelete, ownPath, firstToken, nil, 204, nil)
		p0Call(t, router, http.MethodDelete, ownPath, firstToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, ownPath, firstToken, nil, 404, nil)
		p0Call(t, router, http.MethodGet, path+"/"+foreign.ID.String(), secondToken, nil, 200, &loaded)
		if loaded.ID != foreign.ID || loaded.Title != "Second event" {
			t.Fatalf("second user's event changed: %+v", loaded)
		}
	})

	t.Run("invalid and missing personal records", func(t *testing.T) {
		p0Call(t, router, http.MethodPost, "/api/notes", firstToken, map[string]string{"title": "  ", "content": "x"}, 400, nil)
		p0Call(t, router, http.MethodPost, "/api/calendar/events", firstToken, map[string]any{
			"title": "Invalid", "starts_at": "2026-12-01T10:00:00Z", "ends_at": "2026-12-01T09:00:00Z",
		}, 400, nil)
		for _, path := range []string{"/api/notes/", "/api/calendar/events/"} {
			p0Call(t, router, http.MethodGet, path+uuid.NewString(), firstToken, nil, 404, nil)
			p0Call(t, router, http.MethodGet, path+"bad-id", firstToken, nil, 400, nil)
			p0Call(t, router, http.MethodDelete, path+uuid.NewString(), firstToken, nil, 404, nil)
		}
		for _, path := range []string{"/api/notes", "/api/calendar/events"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, authorizedRequest(http.MethodPost, path, `{"title":`, firstToken))
			if w.Code != 400 {
				t.Fatalf("malformed JSON at %s: %d", path, w.Code)
			}
		}
	})
}
