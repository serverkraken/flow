package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// handleNavTreeFragment serves GET /ui/nav/tree — the htmx-loaded sidebar
// project-tree spine.  It lists the owner's active + paused nodes, builds the
// depth-indented tree, and renders NavTree as an HTML fragment.
func (s *Server) handleNavTreeFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	all, err := s.ListNodes.Execute(r.Context(), u.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "nav tree: list nodes failed", "err", err)
		all = nil
	}
	var visible []domain.Node
	for _, n := range all {
		if n.Status == domain.NodeActive || n.Status == domain.NodePaused {
			visible = append(visible, n)
		}
	}
	rows := webui.BuildTree(visible)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NavTree(rows).Render(r.Context(), w)
}
