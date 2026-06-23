package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/i18n"
)

// handleWebStyleguide renders the /ui design-system showcase. It is gated by
// webAuth (mounted in Routes) and resolves the UI locale for i18n strings.
func (s *Server) handleWebStyleguide(w http.ResponseWriter, r *http.Request) {
	ctx := i18n.WithLocale(r.Context(), i18n.Resolve(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = components.StyleguidePage().Render(ctx, w)
}
