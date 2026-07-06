package components

import "context"

// cspNonceKey is the ctx key for the per-request Content-Security-Policy
// nonce (Lesesaal L3 Task 9). Unexported like i18n's localeKey — callers use
// WithCSPNonce/CSPNonce, never the key itself.
type cspNonceKey struct{}

// WithCSPNonce stores the per-request CSP nonce in ctx. Set by httpserver's
// securityHeaders middleware before any page handler runs, so base.templ's
// two inline <script> tags (theme-init, live-timer) can carry a nonce that
// matches the Content-Security-Policy(-Report-Only) header's script-src.
func WithCSPNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, cspNonceKey{}, nonce)
}

// CSPNonce returns the ctx's CSP nonce, or "" if securityHeaders never ran
// (e.g. a unit test rendering a templ component directly without going
// through Server.Routes()) — an empty nonce attribute is harmless since no
// CSP header claiming a matching 'nonce-' value was sent either.
func CSPNonce(ctx context.Context) string {
	n, _ := ctx.Value(cspNonceKey{}).(string)
	return n
}
