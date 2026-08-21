package httpserver

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// contextModeErrKnown reports whether err is one of the two "clean no-op"
// outcomes for a mode POST — an invalid mode string (domain.ErrInvalidDocument,
// belt-and-suspenders with the DB CHECK) or a foreign/unknown doc id
// (ports.ErrDocumentNotFound, Owner-Scope). Neither is a 500; both just skip
// the mutation and fall through to a clean re-render.
func contextModeErrKnown(err error) bool {
	return errors.Is(err, domain.ErrInvalidDocument) || errors.Is(err, ports.ErrDocumentNotFound)
}

// kontextComposeErr is the Kuratieren fragment's inline error line for a
// Compose failure (docs/tags store I/O) — plain hardcoded German, matching
// the existing renderCockpitMain/renderNodeRail convention (webui_cockpit.go)
// rather than minting a new i18n key for a rare backend-failure branch.
const kontextComposeErr = "Kontext konnte nicht geladen werden"

// kontextDataFor builds the Kuratieren page's VM for nodeID (Owner-Scope: a
// foreign/unknown node surfaces ports.ErrNodeNotFound → the callers 404).
func (s *Server) kontextDataFor(r *http.Request, ownerID, nodeID string) (webui.KontextVM, error) {
	ctx := r.Context()
	n, err := s.GetNode.Execute(ctx, ownerID, nodeID)
	if err != nil {
		return webui.KontextVM{}, err
	}
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 12000
	}
	cc, err := s.ComposeContext.ExecuteForNode(ctx, ownerID, n.ID, budget)
	if err != nil {
		return webui.KontextVM{}, err
	}
	vm := webui.BuildKontextVM(n, cc)
	// Screen 07: die Wahl der Lesespalte kommt als ?doc= von der Seite, als
	// ?sel= von den Aktions-POSTs (deren Formularfeld "doc" ist die Ziel-
	// karte der Aktion, nicht die Wahl).
	sel := r.URL.Query().Get("doc")
	if sel == "" {
		sel = r.URL.Query().Get("sel")
	}
	vm.Select(sel)
	if vm.Selected != "" {
		if doc, derr := s.GetDocument.Execute(ctx, ownerID, vm.Selected); derr == nil {
			vm.Lese = webui.BuildKontextLese(cc, doc, s.renderKontextDoc(r, ownerID, doc), s.Clock.Now())
		} else {
			slog.WarnContext(ctx, "kontext: selected doc failed", "docID", vm.Selected, "err", derr)
		}
	}
	return vm, nil
}

// renderKontextDoc rendert die gewählte Karte für die Lesespalte — mit den
// Auflösern der Kartenfläche (Wikilinks über den Bestand des Besitzers,
// Artefakte über die Kette des Registers), aber ohne deren Rückverweise und
// Embedding-Stand. Jeder Auflöser ist abgesichert: fehlt ein Store, bleibt
// der Link unaufgelöst, die Karte erscheint trotzdem.
func (s *Server) renderKontextDoc(r *http.Request, ownerID string, doc domain.Document) template.HTML {
	ctx := r.Context()
	var all []domain.Document
	if s.ListDocuments.Docs != nil {
		all, _ = s.ListDocuments.Execute(ctx, ownerID, nil, nil)
	}
	resolve := func(target string) (string, string, bool) {
		if t, ok := domain.ResolveWikilink(doc, target, all); ok {
			return "/wissen/" + t.ID, t.Title, true
		}
		return "", "", false
	}
	var resolveArtifact webui.ArtifactResolver
	if s.ListArtifacts.Artifacts != nil {
		nodeID := ""
		var chain []domain.Node
		if doc.NodeID != nil {
			nodeID = *doc.NodeID
			chain, _ = s.NodeAncestors.Execute(ctx, ownerID, nodeID)
		}
		if arts, aerr := s.ListArtifacts.Execute(ctx, ownerID, nodeID); aerr == nil {
			resolveArtifact = buildArtifactResolver(chain, arts)
		}
	}
	html, _ := webui.RenderDocument(ctx, doc.Body, resolve, resolveArtifact)
	return html
}

// handleWebKontextLese serves GET /kontext/{id}/lese: the reading column's
// own SSE fragment (#kontext-lese) — reloaded on document.updated so a mode
// change or an edit elsewhere moves its standing line along.
func (s *Server) handleWebKontextLese(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.kontextDataFor(r, u.ID, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		s.webNotFound(w, r)
		return
	}
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.KontextLese(vm).Render(r.Context(), w)
}

// handleWebKontextView serves GET /kontext/{id}: the full Kuratieren page.
func (s *Server) handleWebKontextView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.kontextDataFor(r, u.ID, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		s.webNotFound(w, r)
		return
	}
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.KontextPage(vm).Render(r.Context(), w)
}

// renderKontext writes the #kontext-fragment only (never the full AppShell
// page) — the shared response body for the view handler's own fragment
// re-render path and both mutation handlers, so their hx-swap="outerHTML"
// against #kontext-fragment never nests a page inside the list (Codex-Fund
// #3, mirrors handleWebDocPin's buildDocumentVM/DocumentFragment split).
func (s *Server) renderKontext(w http.ResponseWriter, r *http.Request, vm webui.KontextVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.KontextFragment(vm).Render(r.Context(), w)
}

// handleWebKontextReorder serves POST /kontext/{id}/reorder: swaps doc's
// position with its up/down neighbor in the current Compose ranking, then
// stamps the resulting order as dense descending priorities via
// ReorderContextDocs. Codex-Fund #4 — none of the following are 500s: a
// foreign/unknown node (404, same as the view handler), a Compose error
// (rendered inline as .alert-err instead of swallowed), a doc that fell out
// of the composed context between page-load and click (concurrently deleted
// or no longer eligible — index not found), or ReorderContextDocs hitting
// ErrDocumentNotFound (doc deleted between Compose and the write) — all
// degrade to a clean no-op re-render of the current fragment.
func (s *Server) handleWebKontextReorder(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.PathValue("id")
	_ = r.ParseForm()
	docID := r.FormValue("doc")
	dir := r.FormValue("dir")

	ctx := r.Context()
	n, err := s.GetNode.Execute(ctx, u.ID, nodeID)
	if errors.Is(err, ports.ErrNodeNotFound) {
		s.webNotFound(w, r)
		return
	}
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 12000
	}
	cc, err := s.ComposeContext.ExecuteForNode(ctx, u.ID, n.ID, budget)
	if err != nil {
		vm := webui.BuildKontextVM(n, cc)
		vm.Err = kontextComposeErr
		s.renderKontext(w, r, vm)
		return
	}

	ids := make([]string, len(cc.Ranked))
	k := -1
	for i, it := range cc.Ranked {
		ids[i] = it.Item.ID
		if it.Item.ID == docID {
			k = i
		}
	}
	if k >= 0 {
		switch {
		case dir == "up" && k > 0:
			ids[k], ids[k-1] = ids[k-1], ids[k]
		case dir == "down" && k < len(ids)-1:
			ids[k], ids[k+1] = ids[k+1], ids[k]
		}
		if err := s.ReorderContextDocs.Execute(ctx, u.ID, ids); err != nil && !errors.Is(err, ports.ErrDocumentNotFound) {
			s.webServerError(w, r, err)
			return
		}
		s.emitEvent(ctx, domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"reordered": true}})
	}

	vm, err := s.kontextDataFor(r, u.ID, nodeID)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	s.renderKontext(w, r, vm)
}

// handleWebKontextPin serves POST /kontext/{id}/pin: toggles a document's
// pinned state (reuses SetPinned, same as handleWebDocPin) and re-renders
// the Kuratieren fragment instead of the document page's own fragment. A
// foreign/unknown doc id degrades to a no-op (SetPinned's ErrDocumentNotFound
// is swallowed, no event emitted) rather than a 500 — mirrors the reorder
// handler's no-op-on-missing-doc behavior.
func (s *Server) handleWebKontextPin(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.PathValue("id")
	_ = r.ParseForm()
	doc := r.FormValue("doc")

	d, _ := s.GetDocument.Execute(r.Context(), u.ID, doc)
	if err := s.SetPinned.Execute(r.Context(), u.ID, doc, !d.Pinned); err == nil {
		s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": doc}})
	} else if !errors.Is(err, ports.ErrDocumentNotFound) {
		s.webServerError(w, r, err)
		return
	}

	vm, err := s.kontextDataFor(r, u.ID, nodeID)
	if errors.Is(err, ports.ErrNodeNotFound) {
		s.webNotFound(w, r)
		return
	}
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	s.renderKontext(w, r, vm)
}

// handleWebKontextMode serves POST /kontext/{id}/mode: the Auto/Immer/Nie
// mode switcher on every Kuratieren row (rang list, Always-Tier, and the new
// Ausgeblendet section, Task 4). ErrInvalidDocument (bad mode string,
// belt-and-suspenders with the DB CHECK) and ErrDocumentNotFound (foreign or
// unknown doc — Owner-Scope) both degrade to a clean no-op re-render, never a
// 500 — mirrors handleWebKontextPin's no-op-on-missing-doc shape.
func (s *Server) handleWebKontextMode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.PathValue("id")
	_ = r.ParseForm()
	doc := r.FormValue("doc")
	mode := r.FormValue("mode")

	if err := s.SetContextMode.Execute(r.Context(), u.ID, doc, domain.ContextMode(mode)); err == nil {
		s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": doc, "action": "context." + mode}})
	} else if !contextModeErrKnown(err) {
		s.webServerError(w, r, err)
		return
	}

	vm, err := s.kontextDataFor(r, u.ID, nodeID)
	if errors.Is(err, ports.ErrNodeNotFound) {
		s.webNotFound(w, r)
		return
	}
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	s.renderKontext(w, r, vm)
}
