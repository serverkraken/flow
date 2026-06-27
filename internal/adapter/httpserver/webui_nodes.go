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

