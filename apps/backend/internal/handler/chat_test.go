package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChat(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Error("wrong upstream route")
		}
		send(w, http.StatusOK, chatResponse{
			Answer:  "demo",
			Sources: []source{},
			Mode:    "mock",
		})
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)

	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"message":"отпуск"}`, 200},
		{`{"message":" "}`, 400},
		{`{"message":"x","extra":1}`, 400},
		{`{"message":"x"} {}`, 400},
		{`broken`, 400},
		{`{"message":"` + strings.Repeat("a", 2001) + `"}`, 400},
	} {
		w := httptest.NewRecorder()
		req := authorizedRequest("POST", "/api/chat", tc.body, token)

		handler.ServeHTTP(w, req)

		if w.Code != tc.status {
			t.Errorf("got %d want %d: %s", w.Code, tc.status, tc.body)
		}
	}
}

// The legacy /api/chat must keep working when the AI service runs with a real model (mode=llm), not only in mock mode.
func TestChatAcceptsLLMMode(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, http.StatusOK, chatResponse{Answer: "ответ модели", Sources: []source{}, Mode: "llm"})
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/chat", `{"message":"hi"}`, token))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"mode":"llm"`) {
		t.Fatalf("mode was not relayed: %s", w.Body.String())
	}
}

func TestChatRequiresAuthentication(t *testing.T) {
	handler, _, _ := newTestHandler(t, "")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/chat",
		strings.NewReader(`{"message":"hello"}`),
	)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAIUnavailable(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)

	w := httptest.NewRecorder()
	req := authorizedRequest(
		http.MethodPost,
		"/api/chat",
		`{"message":"hi"}`,
		token,
	)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestInvalidAIResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "malformed JSON", status: http.StatusOK, body: `{"answer":`},
		{name: "trailing JSON", status: http.StatusOK, body: `{"answer":"ok","sources":[],"mode":"mock"}{}`},
		{name: "wrong mode", status: http.StatusOK, body: `{"answer":"ok","sources":[],"mode":"live"}`},
		{name: "missing sources", status: http.StatusOK, body: `{"answer":"ok","mode":"mock"}`},
		{name: "oversized", status: http.StatusOK, body: `{"answer":"` + strings.Repeat("a", 65536) + `","sources":[],"mode":"mock"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ai.Close()

			handler, token, _ := newTestHandler(t, ai.URL)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, authorizedRequest(http.MethodPost, "/api/chat", `{"message":"hi"}`, token))
			if w.Code != http.StatusBadGateway {
				t.Fatalf("got %d, want %d", w.Code, http.StatusBadGateway)
			}
		})
	}
}

func TestAITimeoutUsesRequestContext(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer ai.Close()

	handler, token, _ := newTestHandler(t, ai.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := authorizedRequest(http.MethodPost, "/api/chat", `{"message":"hi"}`, token).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want %d", w.Code, http.StatusBadGateway)
	}
}
