package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func (s *Server) handleWebDocumentView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	vm, err := s.buildDocumentVM(r, u.ID, doc)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentPage(vm).Render(r.Context(), w)
}

// buildDocumentVM builds the Lesesaal DocumentVM shared by the full page view
// (handleWebDocumentView) and the Anpinnen round-trip (handleWebDocPin),
// which both render the same #document-fragment.
func (s *Server) buildDocumentVM(r *http.Request, ownerID string, doc domain.Document) (webui.DocumentVM, error) {
	all, err := s.ListDocuments.Execute(r.Context(), ownerID, nil, nil)
	if err != nil {
		return webui.DocumentVM{}, err
	}
	resolve := func(target string) (string, string, bool) {
		if t, ok := domain.ResolveWikilink(doc, target, all); ok {
			return "/wissen/" + t.ID, t.Title, true
		}
		return "", "", false
	}
	rendered, _ := webui.RenderDocument(r.Context(), doc.Body, resolve)

	vm := webui.DocumentVM{
		ID:            doc.ID,
		Title:         doc.Title,
		Path:          doc.Path,
		HTML:          rendered,
		UpdatedByKind: doc.UpdatedByKind,
		UpdatedByRef:  doc.UpdatedByRef,
		UpdatedRel:    webui.FmtRelTime(doc.UpdatedAt, s.Clock.Now()),
		ReadMinutes:   webui.ReadingTime(doc.Body),
		Pinned:        doc.Pinned,
	}
	if doc.NodeID != nil {
		// Fire-and-forget like webui_cockpit.go's own CockpitHead spine: an
		// ancestor-lookup failure degrades to "no crumbs but the page still
		// renders" rather than a hard 500 for a purely decorative breadcrumb.
		chain, _ := s.NodeAncestors.Execute(r.Context(), ownerID, *doc.NodeID)
		for i := len(chain) - 1; i >= 0; i-- {
			n := chain[i]
			vm.Crumbs = append(vm.Crumbs, webui.DocCrumb{Label: n.Name, Href: "/nodes/" + n.ID})
		}
	}
	if s.GetEmbedStatus.Docs != nil {
		if st, serr := s.GetEmbedStatus.Execute(r.Context(), ownerID, doc.ID); serr == nil {
			vm.Embed = &webui.EmbedView{
				State:     string(st.State),
				LastError: truncateError(st.LastError),
				ShowRetry: st.State == domain.EmbedFailed,
			}
		}
	}
	return vm, nil
}

func (s *Server) handleWebDocReembed(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.RetryEmbedding.Execute(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, "/wissen/"+id, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentEmbedBadge(id, webui.EmbedView{State: "pending"}).Render(r.Context(), w)
}

// handleWebDocPin toggles a document's pinned state (POST /wissen/{id}/pin —
// the Anpinnen button in the Provenance row), emits document.updated so the
// #document-fragment SSE-refreshes everywhere else the doc is open, and
// returns the fresh fragment for the button's own hx-swap="outerHTML".
func (s *Server) handleWebDocPin(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := s.SetPinned.Execute(r.Context(), u.ID, id, !doc.Pinned); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.Pinned = !doc.Pinned
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})

	vm, err := s.buildDocumentVM(r, u.ID, doc)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentFragment(vm).Render(r.Context(), w)
}

// wissenCategoryHrefAndLabel is no longer used by the document page itself
// (Lesesaal Spine replaces the old "Zurück zu Kategorie" breadcrumb), but
// webui_editor.go still uses it for the editor's own "back to category"
// link — kept for that caller (Bestand gewinnt; rg-verified).
func wissenCategoryHrefAndLabel(doc domain.Document) (string, string) {
	if cat, ok := webui.WissenCategoryForType(doc.Type); ok {
		return cat.Href, cat.LabelKey
	}
	return "/wissen", "wissen.title"
}

func truncateError(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
