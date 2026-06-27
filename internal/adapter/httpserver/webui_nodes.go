package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// nodesListData loads the owner's nodes, applies the status filter and builds
// the indented tree.  "" → active+paused; "archived" → archived only; "all".
func (s *Server) nodesListData(r *http.Request, u domain.User) webui.NodesPageData {
	status := r.URL.Query().Get("status")
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	filtered := make([]domain.Node, 0, len(all))
	for _, n := range all {
		switch status {
		case "all":
			filtered = append(filtered, n)
		case "archived":
			if n.Status == domain.NodeArchived {
				filtered = append(filtered, n)
			}
		default: // active + paused
			if n.Status == domain.NodeActive || n.Status == domain.NodePaused {
				filtered = append(filtered, n)
			}
		}
	}
	return webui.NodesPageData{
		User:   u.Username,
		Status: status,
		Rows:   webui.BuildTree(filtered),
	}
}

// handleWebNodeTree renders the node tree fragment at GET /ui/nodes/tree.
// It is the SSE-swap target referenced by the <div hx-get="/ui/nodes/list">
// outer wrapper; an identical fragment is also served at /ui/nodes/list via
// handleWebNodesList.
func (s *Server) handleWebNodeTree(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodesFragment(s.nodesListData(r, u)).Render(r.Context(), w)
}
