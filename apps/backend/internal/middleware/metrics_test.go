package middleware

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestHTTPMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok") // implicit 200
	})
	mux.HandleFunc("GET /failed", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := InstrumentHTTP(mux)
	if strings.Contains(scrape(t, h), "http_requests_total{") {
		t.Fatal("metrics scrape counted itself")
	}

	for _, tc := range []struct {
		path string
		code int
	}{
		{"/api/items/7e83266f-b3ee-41e4-ac69-9e046d8eb65c?secret=ignored", 200},
		{"/failed", 500},
		{"/empty", 204},
		{"/missing/another-uuid", 404},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.code {
			t.Errorf("%s: status = %d, want %d", tc.path, w.Code, tc.code)
		}
	}

	metrics := scrape(t, h)
	for _, want := range []string{
		`http_requests_total{method="GET",route="GET /api/items/{id}",status="200"} 1`,
		`http_requests_total{method="GET",route="GET /failed",status="500"} 1`,
		`http_requests_total{method="GET",route="GET /empty",status="204"} 1`,
		`http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`http_request_duration_seconds_count{method="GET",route="GET /api/items/{id}"} 1`,
		`http_requests_in_flight 0`,
		`go_goroutines`,
		`process_cpu_seconds_total`,
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("missing metric %q", want)
		}
	}
	for _, forbidden := range []string{"7e83266f", "secret=ignored", "/missing/another-uuid"} {
		if strings.Contains(metrics, forbidden) {
			t.Errorf("high-cardinality value leaked: %q", forbidden)
		}
	}
}

func TestMetricsRegistryIsPerRouter(t *testing.T) {
	for range 3 {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {})
		h := InstrumentHTTP(mux)
		if strings.Contains(scrape(t, h), "http_requests_total{") {
			t.Fatal("new router inherited request metrics")
		}
	}
}

func TestResponseWriterCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("Flush capability lost")
		}
		if _, ok := w.(io.ReaderFrom); ok {
			t.Error("ReaderFrom incorrectly advertised")
		}
		if _, ok := w.(http.Hijacker); ok {
			t.Error("Hijack incorrectly advertised")
		}
		if _, ok := w.(http.Pusher); ok {
			t.Error("Push incorrectly advertised")
		}
	})
	InstrumentHTTP(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

type capableWriter struct {
	*httptest.ResponseRecorder
	flushed bool
	pushed  bool
	read    bool
}

func (w *capableWriter) Flush() { w.flushed = true }
func (w *capableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("test hijack")
}
func (w *capableWriter) Push(string, *http.PushOptions) error {
	w.pushed = true
	return nil
}
func (w *capableWriter) ReadFrom(r io.Reader) (int64, error) {
	w.read = true
	return io.Copy(w.ResponseRecorder, r)
}

func TestSupportedResponseWriterCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, _, _ = w.(http.Hijacker).Hijack()
		_ = w.(http.Pusher).Push("/next", nil)
		_, _ = w.(io.ReaderFrom).ReadFrom(strings.NewReader("body"))
	})
	w := &capableWriter{ResponseRecorder: httptest.NewRecorder()}
	InstrumentHTTP(mux).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !w.flushed || !w.pushed || !w.read || w.Body.String() != "body" {
		t.Fatalf("optional writer capabilities changed: %+v", w)
	}
}
