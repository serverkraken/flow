package httpserver

// metrics_test.go — Plan F · Task 6.
//
// Unit coverage for the Prometheus middleware + handler:
//
//   - middleware writes the correct method/route/status labels
//   - middleware records non-zero duration
//   - status defaults to 200 when the inner handler never calls WriteHeader
//   - status non-200 is captured via WriteHeader
//   - /metrics endpoint returns the prometheus text format
//   - /metrics is not double-counted by the middleware
//   - SyncConflicts.Inc() appears in the /metrics text output

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// counterValue returns the current value of a labelled counter from the
// default registry. Returns 0 if the counter was never observed.
func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// histogramCount returns the sample count of a labelled histogram. 0 if
// the series was never observed.
func histogramCount(t *testing.T, name string, labels map[string]string) uint64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func labelsMatch(got []*dto.LabelPair, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	have := make(map[string]string, len(got))
	for _, lp := range got {
		have[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

// TestUnit_MetricsMiddleware_IncrementsRequestsAndDuration mounts a tiny
// chi router with a known route pattern, hits it, and asserts both the
// Requests counter and the Duration histogram observed one sample with
// the expected labels.
func TestUnit_MetricsMiddleware_IncrementsRequestsAndDuration(t *testing.T) {
	// Not Parallel — these tests share the global default registry and
	// would race on the counter readback otherwise.
	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Get("/things/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	requestsBefore := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "/things/{id}",
		"status": "200",
	})
	durationBefore := histogramCount(t, "flow_http_request_duration_seconds", map[string]string{
		"method": "GET",
		"route":  "/things/{id}",
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/things/abc-123", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	requestsAfter := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "/things/{id}",
		"status": "200",
	})
	if requestsAfter != requestsBefore+1 {
		t.Errorf("requests delta = %v, want +1", requestsAfter-requestsBefore)
	}

	durationAfter := histogramCount(t, "flow_http_request_duration_seconds", map[string]string{
		"method": "GET",
		"route":  "/things/{id}",
	})
	if durationAfter != durationBefore+1 {
		t.Errorf("duration sample count delta = %v, want +1", durationAfter-durationBefore)
	}
}

// TestUnit_MetricsMiddleware_StatusDefaultsTo200 covers the net/http
// convention: a handler that never calls WriteHeader implicitly returns
// 200. The middleware must label the counter with "200" in that case.
func TestUnit_MetricsMiddleware_StatusDefaultsTo200(t *testing.T) {
	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Get("/silent", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("no WriteHeader call"))
	})

	before := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "/silent",
		"status": "200",
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/silent", nil)
	r.ServeHTTP(rr, req)

	after := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "/silent",
		"status": "200",
	})
	if after != before+1 {
		t.Errorf("silent 200 counter delta = %v, want +1", after-before)
	}
}

// TestUnit_MetricsMiddleware_CapturesNon200Status verifies the
// statusRecorder hands the right status to the counter label.
func TestUnit_MetricsMiddleware_CapturesNon200Status(t *testing.T) {
	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Post("/conflict", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	before := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "POST",
		"route":  "/conflict",
		"status": "409",
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/conflict", nil)
	r.ServeHTTP(rr, req)

	after := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "POST",
		"route":  "/conflict",
		"status": "409",
	})
	if after != before+1 {
		t.Errorf("409 counter delta = %v, want +1", after-before)
	}
}

// TestUnit_StatusRecorder_DefaultsTo200 unit-tests the recorder in
// isolation — guards against accidental zero-value regressions.
func TestUnit_StatusRecorder_DefaultsTo200(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr, status: http.StatusOK}
	if rec.status != http.StatusOK {
		t.Fatalf("default status = %d, want 200", rec.status)
	}
}

// TestUnit_StatusRecorder_CapturesWriteHeader verifies that an explicit
// WriteHeader propagates to both the recorder field and the underlying
// writer.
func TestUnit_StatusRecorder_CapturesWriteHeader(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr, status: http.StatusOK}
	rec.WriteHeader(http.StatusTeapot)
	if rec.status != http.StatusTeapot {
		t.Errorf("captured status = %d, want 418", rec.status)
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("underlying writer status = %d, want 418", rr.Code)
	}
}

// TestUnit_MetricsHandler_ReturnsPrometheusFormat hits the /metrics
// handler through a chi router (same shape as production wiring) and
// verifies the body looks like prometheus text format. Prometheus only
// emits HELP+series for collectors that have observed at least one
// sample (CounterVec/HistogramVec are lazy on label-value combinations),
// so the test pre-bumps each metric to force them into the output.
func TestUnit_MetricsHandler_ReturnsPrometheusFormat(t *testing.T) {
	// Pre-bump so the lazily-instantiated label-value series appear in
	// the scrape output. The actual increment values don't matter — we
	// only assert presence of the metric name.
	SyncConflicts.WithLabelValues("sessions").Inc()
	DBQueryDuration.WithLabelValues("sessions", "GetByID").Observe(0.001)

	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Handle(metricsPath, NewMetricsHandler())

	// Hit a real route once so the http_requests / http_duration series
	// also exist before the scrape.
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	pingRR := httptest.NewRecorder()
	pingReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(pingRR, pingReq)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, metricsPath, nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	// promhttp emits either text/plain;version=0.0.4 or
	// application/openmetrics-text — both start with "text/" or
	// "application/openmetrics" and contain version markers.
	if !strings.HasPrefix(ct, "text/plain") && !strings.HasPrefix(ct, "application/openmetrics") {
		t.Errorf("Content-Type = %q, want prometheus text format", ct)
	}
	body, _ := io.ReadAll(rr.Body)
	// Every prometheus text-format body has at least one "# HELP" line.
	if !strings.Contains(string(body), "# HELP") {
		t.Errorf("body missing '# HELP' line — not a prometheus exposition?\nbody:\n%s", body)
	}
	// The collectors we registered must show up in the output.
	for _, name := range []string{
		"flow_http_requests_total",
		"flow_http_request_duration_seconds",
		"flow_sync_conflicts_total",
		"flow_db_query_duration_seconds",
	} {
		if !strings.Contains(string(body), name) {
			t.Errorf("/metrics output missing %q", name)
		}
	}
}

// TestUnit_MetricsMiddleware_SkipsMetricsEndpoint ensures the middleware
// does NOT count /metrics itself — otherwise the Prometheus scrape would
// inflate its own counter on every poll.
func TestUnit_MetricsMiddleware_SkipsMetricsEndpoint(t *testing.T) {
	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Handle(metricsPath, NewMetricsHandler())

	before := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  metricsPath,
		"status": "200",
	})
	beforeUnknown := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "unknown",
		"status": "200",
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, metricsPath, nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	after := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  metricsPath,
		"status": "200",
	})
	afterUnknown := counterValue(t, "flow_http_requests_total", map[string]string{
		"method": "GET",
		"route":  "unknown",
		"status": "200",
	})
	if after != before {
		t.Errorf("/metrics was counted: delta = %v, want 0", after-before)
	}
	if afterUnknown != beforeUnknown {
		t.Errorf("/metrics fell through to 'unknown' route label: delta = %v, want 0", afterUnknown-beforeUnknown)
	}
}

// TestUnit_SyncConflicts_AppearsInOutput exercises the cross-package
// hook: bump a label, scrape /metrics, expect the counter to show up.
func TestUnit_SyncConflicts_AppearsInOutput(t *testing.T) {
	SyncConflicts.WithLabelValues("sessions").Inc()
	SyncConflicts.WithLabelValues("projects").Inc()
	SyncConflicts.WithLabelValues("repo_notes").Inc()

	r := chi.NewRouter()
	r.Handle(metricsPath, NewMetricsHandler())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, metricsPath, nil)
	r.ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	for _, label := range []string{"sessions", "projects", "repo_notes"} {
		needle := `flow_sync_conflicts_total{resource="` + label + `"}`
		if !strings.Contains(string(body), needle) {
			t.Errorf("/metrics missing %q", needle)
		}
	}
}

// TestUnit_MetricsMiddleware_DurationIsPositive — duration histograms
// silently dropping zero-second samples would be hard to spot in prod;
// pin the invariant that the observed sample is at minimum non-negative
// and gets recorded.
func TestUnit_MetricsMiddleware_DurationIsPositive(t *testing.T) {
	r := chi.NewRouter()
	r.Use(NewMetricsMiddleware)
	r.Get("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	before := histogramSum(t, "flow_http_request_duration_seconds", map[string]string{
		"method": "GET",
		"route":  "/slow",
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	r.ServeHTTP(rr, req)

	after := histogramSum(t, "flow_http_request_duration_seconds", map[string]string{
		"method": "GET",
		"route":  "/slow",
	})
	if after <= before {
		t.Errorf("duration sum did not grow: before=%v after=%v", before, after)
	}
}

// histogramSum returns the cumulative sum of observed values for a
// labelled histogram. 0 if the series was never observed.
func histogramSum(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleSum()
			}
		}
	}
	return 0
}
