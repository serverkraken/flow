package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// nodeFormValues reads all node form fields from the request.
func nodeFormValues(r *http.Request) webui.NodeFormValues {
	return webui.NodeFormValues{
		Name:         r.FormValue("name"),
		Slug:         r.FormValue("slug"),
		Kind:         r.FormValue("kind"),
		ParentID:     r.FormValue("parentId"),
		Description:  r.FormValue("description"),
		UpstreamGit:  r.FormValue("upstreamGit"),
		Status:       r.FormValue("status"),
		Color:        r.FormValue("color"),
		Glyph:        r.FormValue("glyph"),
		RateAmount:   r.FormValue("rateAmount"),
		RateCurrency: r.FormValue("rateCurrency"),
	}
}

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

