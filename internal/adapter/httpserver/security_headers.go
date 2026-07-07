package httpserver

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// cspHeaderName picks the enforcing header once the L3 Task 9/10 smoke shows
// zero violations (Soenne Entsch. #8); until then (default, CSPEnforce
// false) violations are only reported, never blocked.
func cspHeaderName(enforce bool) string {
	if enforce {
		return "Content-Security-Policy"
	}
	return "Content-Security-Policy-Report-Only"
}

// cspPolicy builds the L3-Task-9 policy string for a single request's nonce.
// style-src keeps 'unsafe-inline' deliberately (Mermaid-SVG inline styles,
// templ style= attributes such as ColorHex/wocheDayBarStyle/heuteBarStyle —
// Offene Entsch. #8, l3-global-constraints.md).
func cspPolicy(nonce string) string {
	return "default-src 'self'; " +
		"script-src 'self' 'nonce-" + nonce + "'; " +
		"script-src-attr 'none'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'none'; " +
		"img-src 'self' data:; " +
		"style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; " +
		"font-src 'self'"
}

// newCSPNonce returns a fresh, per-request base64 nonce for CSP script-src.
func newCSPNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// securityHeaders is the outermost layer of the web-serving chain (Lesesaal
// L3 Task 9): agent-authored Markdown is untrusted content (Spec §11), and
// the goldmark-no-html + bluemonday sanitizer boundary (Task 2) gets a
// defense-in-depth backstop here. It mints a per-request nonce, stashes it
// in ctx for base.templ's two inline <script> tags (theme-init, live-timer),
// and sets the CSP header — Report-Only by default (s.CSPEnforce == false)
// until a smoke across every surface (document/Mermaid, Wissen search,
// palette, SSE, dialogs, editor preview, projects/cockpit/time) shows zero
// violations. Applied to every response, not just cookie-gated pages, so
// pre-auth pages (login error, logged-out) that render the same base.templ
// hull also get a matching nonce.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := newCSPNonce()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set(cspHeaderName(s.CSPEnforce), cspPolicy(nonce))
		next.ServeHTTP(w, r.WithContext(components.WithCSPNonce(r.Context(), nonce)))
	})
}
