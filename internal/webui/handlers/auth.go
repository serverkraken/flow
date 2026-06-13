// Package handlers wires the templ-generated templates to net/http handlers.
// Each .go file is one route group (auth, dashboard, worktime, …) so the
// composition root in cmd/flow-server stays a thin index.
package handlers

import (
	"net/http"

	"github.com/serverkraken/flow/internal/webui/templates/auth"
)

// AuthDeps holds the (small) bag of values the auth-landing handler needs.
// `IssuerLabel` is shown on the page so the user knows which IdP they'll
// be redirected to ("authentik.example.com", "dex (local dev)", etc.).
type AuthDeps struct {
	IssuerLabel string
}

// NewLanding renders the unauthenticated landing page. Mounted at
// httpserver.LandingPath so the BrowserAuthMiddleware can redirect to it.
func NewLanding(d AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := auth.Login(d.IssuerLabel).Render(r.Context(), w); err != nil {
			http.Error(w, "render", http.StatusInternalServerError)
		}
	})
}
