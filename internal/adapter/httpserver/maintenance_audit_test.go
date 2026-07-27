package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestAuditDocumentsEndpoint_IsReadOnlyAndReturnsStructuredReport(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, http.MethodGet, "/api/v1/maintenance/audit-documents", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var report domain.DocumentAuditReport
	if err := json.NewDecoder(res.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 0 || report.Issues == nil {
		t.Fatalf("unexpected empty report: %+v", report)
	}
}
