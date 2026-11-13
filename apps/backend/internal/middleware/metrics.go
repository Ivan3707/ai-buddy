package middleware

import (
	"net/http"
	"strconv"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InstrumentHTTP keeps all collectors local to a router instance.
func InstrumentHTTP(mux *http.ServeMux) http.Handler {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests handled by the backend.",
	}, []string{"method", "route", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Backend HTTP request duration in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15},
	}, []string{"method", "route"})
	inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current backend HTTP requests in flight, excluding metrics scrapes.",
	})
	registry.MustRegister(requests, duration, inFlight,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			mux.ServeHTTP(w, r)
			return
		}

		// ServeMux sets r.Pattern only while routing. Handler returns the same
		// registered pattern before serving, including paths with variables.
		_, route := mux.Handler(r)
		if route == "" {
			route = "unmatched"
		}

		inFlight.Inc()
		defer inFlight.Dec()
		result := httpsnoop.CaptureMetrics(mux, w, r)
		requests.WithLabelValues(r.Method, route, strconv.Itoa(result.Code)).Inc()
		duration.WithLabelValues(r.Method, route).Observe(result.Duration.Seconds())
	})
}
