package httpserver

// metrics.go — Plan F · Task 6.
//
// Prometheus instrumentation for the HTTP surface:
//
//   - flow_http_requests_total{method,route,status}        — counter
//   - flow_http_request_duration_seconds{method,route}     — histogram
//   - flow_sync_conflicts_total{resource}                  — counter (OCC 409s)
//   - flow_db_query_duration_seconds{adapter,method}       — histogram
//   - flow_sse_subscribers                                 — gauge (live)
//
// The collectors register into prometheus's default registry on package
// init via promauto, so cmd/flow-server/main.go does not need to wire
// anything — the /metrics endpoint mounted by NewMetricsHandler scrapes
// the same default registry.
//
// /metrics is intentionally NOT wrapped in cookie- or bearer-auth. The
// Prometheus scrape model expects anonymous pull; access control belongs
// at the network layer (NetworkPolicy, ServiceMesh, ingress allowlist).
// NewMetricsMiddleware skips /metrics itself to avoid self-observation
// noise (the scrape would otherwise show up as an ever-growing counter
// on its own route).

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collectors. Exported so peer packages (webui handlers, sse broadcaster,
// sql adapters) can increment them without a layering inversion.
// Prometheus collectors are package-global by design — promauto registers
// into the default registry at init.
//
//nolint:gochecknoglobals
var (
	// HTTPRequests counts every HTTP request that reached a handler,
	// labelled by method, chi route pattern, and response status.
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flow_http_requests_total",
		Help: "HTTP requests by method, route, status.",
	}, []string{"method", "route", "status"})

	// HTTPDuration records wall-clock time spent in the handler chain
	// per (method, route). Uses prometheus default buckets which cover
	// the 5ms…10s range typical of a CRUD JSON/HTML app.
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "flow_http_request_duration_seconds",
		Help:    "HTTP request duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	// SyncConflicts counts the OCC version-mismatch 409s the server
	// surfaces to clients. Labelled by resource so the dashboard can
	// spot a wedged single-resource sync loop. Resources: sessions,
	// projects, active, repos, repo_notes.
	SyncConflicts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flow_sync_conflicts_total",
		Help: "OCC conflicts surfaced to clients, by resource.",
	}, []string{"resource"})

	// DBQueryDuration records SQL query latency. Reserved for the
	// sqlite adapters to instrument their hot paths in Plan F follow-up
	// tasks — the collector is registered now so the metric exists
	// the moment the first adapter wires it in (avoids "metric appears
	// at minute 30" dashboard gaps).
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "flow_db_query_duration_seconds",
		Help:    "SQL query duration by adapter+method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"adapter", "method"})
)

// NewMetricsHandler returns the promhttp handler that serves the
// Prometheus text exposition format. Mount at /metrics OUTSIDE any auth
// group — the scrape pulls anonymously.
func NewMetricsHandler() http.Handler {
	return promhttp.Handler()
}

// metricsPath is the URL we DON'T want to self-observe. Hard-coded
// rather than passed in because there's exactly one Prometheus endpoint
// per process and varying it would create label cardinality drift
// across deployments.
const metricsPath = "/metrics"

// NewMetricsMiddleware wraps an http.Handler with request-count and
// duration instrumentation. Skips /metrics itself so the scrape doesn't
// inflate its own counter. Status defaults to 200 if the inner handler
// never calls WriteHeader (Go's net/http convention).
//
// Route label comes from chi.RouteContext().RoutePattern() so it stays
// LOW cardinality (e.g. "/api/v1/sessions/{id}" instead of every UUID).
// Falls back to "unknown" if the request didn't match any registered
// route — typically 404s on unknown paths — to keep the label set bounded.
func NewMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip instrumentation for /metrics itself. The scrape would
		// otherwise produce a self-incrementing counter that drowns
		// out signal in dashboards.
		if r.URL.Path == metricsPath {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// chi.RouteContext is nil for any handler mounted directly on
		// the router without going through a chi.Mux match — defensive
		// fallback keeps the label set bounded.
		route := "unknown"
		if rc := chi.RouteContext(r.Context()); rc != nil {
			if p := rc.RoutePattern(); p != "" {
				route = p
			}
		}
		HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder is a minimal http.ResponseWriter wrapper that captures
// the status code so the metrics middleware can label by it. Go's
// net/http convention: a handler that never calls WriteHeader implicitly
// returns 200 on the first Write — we default to 200 so that case is
// labelled correctly.
//
// Flush and Hijack are forwarded explicitly so handlers that probe for
// http.Flusher (SSE endpoint at /api/v1/events) or http.Hijacker
// (websocket-style upgrades) still find them after the wrap. Without
// these passthroughs, the SSE handler responds 500 ("streaming not
// supported") because the type assertion fails.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code and forwards to the underlying
// writer. Must NOT call WriteHeader twice (net/http logs a warning); the
// status field is the single source of truth for the metrics label.
func (r *statusRecorder) WriteHeader(s int) {
	r.status = s
	r.ResponseWriter.WriteHeader(s)
}

// Flush forwards to the underlying writer if it implements http.Flusher.
// Required for SSE streaming. Silently no-ops if the underlying writer
// doesn't support flushing — same shape as net/http's wrapper writers.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards to the underlying writer if it implements
// http.Hijacker. Required for any connection-takeover paths (websockets,
// raw TCP). Returns an error if the underlying writer doesn't support
// it.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
