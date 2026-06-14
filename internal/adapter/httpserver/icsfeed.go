package httpserver

import (
	"bytes"
	"errors"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
)

// handleIcsFeed is the third auth path: a secret token in the URL path, no
// OIDC/cookie. Unknown or revoked tokens return 404 (no existence leak).
//
// The route is registered as GET /ics/{token} because Go's ServeMux requires
// wildcards to be full path segments. The segment includes the ".ics" suffix
// that icsURL appends, so we strip it here before resolving.
func (s *Server) handleIcsFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("token"), ".ics")
	var buf bytes.Buffer
	err := s.IcsFeed.Execute(r.Context(), token, &buf)
	if errors.Is(err, ports.ErrFeedTokenNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(buf.Bytes())
}
