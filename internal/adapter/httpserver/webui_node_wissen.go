package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// nodeWissenData lädt, was der Wissens-Überblick je Ebene braucht: das
// Register, seine Kette (Brotkrume), den Teilbaum (Herkünfte) und die aktiven
// Karten des Besitzers. Alles owner-scoped; ein fehlender Teilbaum oder eine
// fehlgeschlagene Kartenliste degradiert zur leeren Fläche, nie zum 500.
func (s *Server) nodeWissenData(r *http.Request, u domain.User, id string) (webui.NodeCockpit, webui.WissenEbeneVM, error) {
	ctx := r.Context()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, webui.WissenEbeneVM{}, err
	}
	crumb := webui.NodeCockpit{User: u.Username, N: n}
	crumb.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)

	in := webui.WissenEbeneInput{N: n, Now: s.Clock.Now(), Query: webui.ParseWissenEbeneQuery(r.URL.Query())}
	if n.Kind != domain.KindRepo && s.Stats.Nodes != nil {
		if sub, serr := s.Stats.Nodes.Subtree(ctx, u.ID, n.ID); serr == nil {
			in.Subtree = sub
		} else {
			slog.WarnContext(ctx, "wissen ebene: subtree failed", "nodeID", n.ID, "err", serr)
		}
	}
	if s.ListDocuments.Docs != nil {
		if docs, derr := s.ListDocuments.Execute(ctx, u.ID, nil, nil); derr == nil {
			in.Docs = docs
		} else {
			slog.WarnContext(ctx, "wissen ebene: list docs failed", "nodeID", n.ID, "err", derr)
		}
	}
	scope := "subtree"
	if n.Kind == domain.KindRepo {
		scope = "self"
	}
	in.ArchivedTotal = s.wissenArchivedCount(ctx, u.ID, n, scope)
	return crumb, webui.BuildWissenEbene(ctx, in), nil
}

// handleWebNodeWissen bedient GET /nodes/{id}?tab=wissen (die Seite) und
// GET /nodes/{id}/wissen (das Fragment für htmx und SSE). Ein htmx-Aufruf
// bekommt nur das Fragment; eine Browser-Navigation auf die Fragment-Route
// bekommt die ganze Seite — ein Fragment ohne Stylesheet wäre sonst die
// Antwort auf einen Reload.
func (s *Server) handleWebNodeWissen(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	crumb, vm, err := s.nodeWissenData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") != "" {
		_ = webui.WissenEbeneFragment(vm).Render(r.Context(), w)
		return
	}
	_ = webui.NodeWissenPage(crumb, vm).Render(r.Context(), w)
}
