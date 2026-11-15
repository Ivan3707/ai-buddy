package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyRouteStatus(t *testing.T) {
	cases := []struct {
		name   string
		check  func(context.Context) error
		status int
	}{
		{name: "ready", check: func(context.Context) error { return nil }, status: http.StatusOK},
		{name: "not ready", check: func(context.Context) error { return errors.New("schema is outdated") }, status: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter("http://127.0.0.1:1", Dependencies{Ready: tc.check})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if recorder.Code != tc.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
