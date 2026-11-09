package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const demo = `[{"id":"company","role":"all","tasks":[]},{"id":"ml","role":"ml","tasks":[]},{"id":"backend","role":"backend","tasks":[]}]`

func TestChat(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Error("wrong upstream route")
		}
		send(w, 200, chatResponse{Answer: "demo", Sources: []source{}, Mode: "mock"})
	}))
	defer ai.Close()
	handler := newHandler(ai.URL, []byte(demo))
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"message":"отпуск"}`, 200}, {`{"message":" "}`, 400}, {`{"message":"x","extra":1}`, 400},
		{`{"message":"x"} {}`, 400}, {`broken`, 400}, {`{"message":"` + strings.Repeat("a", 2001) + `"}`, 400},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Errorf("got %d want %d: %s", w.Code, tc.status, tc.body)
		}
	}
}

func TestAIUnavailable(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	handler := newHandler(ai.URL, []byte(demo))
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"message":"hi"}`)))
		if w.Code != 502 {
			t.Errorf("got %d", w.Code)
		}
		ai.Close()
	}
}

func TestRoleFiltering(t *testing.T) {
	handler := newHandler("", []byte(demo))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/onboarding?role=ml", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "backend") || !strings.Contains(w.Body.String(), "company") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/onboarding?role=invalid", nil))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
