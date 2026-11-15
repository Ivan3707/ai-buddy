package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

// fakeAdmin shows the point of the narrow handler interfaces (ARCH-06): the admin routes are tested without
// PostgreSQL or the AI service. Methods the test does not call are left to the embedded nil interface.
type fakeAdmin struct {
	handler.Admin
	open bool
}

func (f fakeAdmin) IsOpenForAll() bool { return f.open }

func (f fakeAdmin) System(context.Context) service.AdminSystem { return service.AdminSystem{} }

func TestAdminRoutesGateWithFakeService(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler { return next } // no signed-in user in the context
	for _, tc := range []struct {
		name         string
		open         bool
		access       string
		systemStatus int
	}{
		{"teamlead only", false, `"allowed":false`, http.StatusUnauthorized},
		{"demo open for all", true, `"allowed":true`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			handler.RegisterAdminRoutes(mux, passThrough, fakeAdmin{open: tc.open})

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/access", nil))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.access) {
				t.Fatalf("access: %d %s", w.Code, w.Body.String())
			}

			w = httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/system", nil))
			if w.Code != tc.systemStatus {
				t.Fatalf("system: got %d want %d: %s", w.Code, tc.systemStatus, w.Body.String())
			}
		})
	}
}
