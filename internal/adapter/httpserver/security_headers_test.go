package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
)

// TestSecurityHeadersReportOnlyByDefault: CSPEnforce's zero value (false) —
// the default until the L3 Task 9/10 smoke proves zero violations (Soenne
// Entsch. #8) — must carry Content-Security-Policy-Report-Only, never the
// enforcing header, with a script-src nonce.
func TestSecurityHeadersReportOnlyByDefault(t *testing.T) {
	srv := &httpserver.Server{Bus: sse.NewBus()}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	csp := res.Header.Get("Content-Security-Policy-Report-Only")
	if csp == "" {
		t.Fatal("expected Content-Security-Policy-Report-Only header")
	}
	if !strings.Contains(csp, "script-src 'self' 'nonce-") {
		t.Fatalf("csp missing nonce script-src: %q", csp)
	}
	if res.Header.Get("Content-Security-Policy") != "" {
		t.Fatal("enforcing CSP header must not be set while CSPEnforce is false")
	}
}

// TestSecurityHeadersNoncesDiffer: two requests must never share a nonce —
// a fixed nonce would let injected inline scripts self-authorize.
func TestSecurityHeadersNoncesDiffer(t *testing.T) {
	srv := &httpserver.Server{Bus: sse.NewBus()}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	nonceOf := func() string {
		t.Helper()
		res, err := http.Get(ts.URL + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		csp := res.Header.Get("Content-Security-Policy-Report-Only")
		i := strings.Index(csp, "nonce-")
		if i < 0 {
			t.Fatalf("no nonce in csp header: %q", csp)
		}
		rest := csp[i+len("nonce-"):]
		j := strings.IndexByte(rest, '\'')
		if j < 0 {
			t.Fatalf("unterminated nonce in csp header: %q", csp)
		}
		return rest[:j]
	}
	n1, n2 := nonceOf(), nonceOf()
	if n1 == "" || n2 == "" {
		t.Fatal("empty nonce")
	}
	if n1 == n2 {
		t.Fatalf("expected different nonces per request, got %q twice", n1)
	}
}

// TestSecurityHeadersEnforceMode: CSPEnforce=true flips to the enforcing
// header (Soenne Entsch. #8, once the smoke is clean).
func TestSecurityHeadersEnforceMode(t *testing.T) {
	srv := &httpserver.Server{Bus: sse.NewBus(), CSPEnforce: true}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("expected enforcing Content-Security-Policy header when CSPEnforce is true")
	}
	if res.Header.Get("Content-Security-Policy-Report-Only") != "" {
		t.Fatal("report-only header must not be set in enforce mode")
	}
}
