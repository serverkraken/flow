package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// handleHomeHome renders the minimal Home landing page at GET /.
// Slice 4 enriches this with a timer-hero, saldo tiles, log-stream,
// and neueste Wissensartikel.
func (s *Server) handleHomeHome(w http.ResponseWriter, r *http.Request) {
	vm := webui.HomeVM{}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HomePage(vm).Render(r.Context(), w)
}
