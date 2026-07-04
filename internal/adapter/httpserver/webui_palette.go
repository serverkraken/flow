package httpserver

import (
	"net/http"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// handlePalette liefert die ⌘K-Sprungzeilen: Knoten (MRU aus den letzten 30
// Tagen Sessions) + jüngste Dokumente, fuzzy gefiltert über q.
func (s *Server) handlePalette(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	owner := u.ID
	q := r.URL.Query().Get("q")

	nodes, err := s.ListNodes.Execute(r.Context(), owner)
	if err != nil {
		http.Error(w, "list nodes", http.StatusInternalServerError)
		return
	}
	sessions, err := s.ListSessions.Execute(r.Context(), owner, s.Clock.Now().Add(-30*24*time.Hour))
	if err != nil {
		sessions = nil // MRU ist Komfort — degradiert still
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Start.After(sessions[j].Start) })
	var recent []string
	seen := map[string]bool{}
	for _, ws := range sessions {
		if ws.NodeID != nil && !seen[*ws.NodeID] {
			seen[*ws.NodeID] = true
			recent = append(recent, *ws.NodeID)
		}
	}
	docs, _, err := s.ListDocumentsPage.Execute(r.Context(), owner, nil, nil, 200, 0)
	if err != nil {
		http.Error(w, "list documents", http.StatusInternalServerError)
		return
	}
	vm := webui.BuildPaletteVM(nodes, recent, docs, q)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.PaletteResults(vm).Render(r.Context(), w)
}
