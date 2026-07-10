package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// handleWebEditorArtefaktePicker serves GET /ui/editor/artefakte
// (?node=/?projectId=&aq=): the "Artefakt einfügen ⋯" toolbar button's
// picker fragment. The SAME endpoint backs both the button's initial hx-get
// (opening the picker) and the picker's own filter input's re-filter
// requests (editor.templ's InsertPickerVM.Endpoint) — the row list, or the
// empty state, is all this returns; the filter shell itself is static markup
// already in the page (components.InsertPicker).
//
// Reuses usecase.ListArtifacts (Task 2/3), so the reachable set is exactly
// the node's Ahnenkette — the same scope the read-side ![[slug]] resolver
// (buildArtifactResolver, webui_document.go) already uses. Owner-scoped end
// to end: NodeStore.Ancestors resolves the chain for THIS owner only (a
// foreign or unknown node id degrades to an empty chain — see
// testutil.FakeNodeStore.Ancestors / pgstore's mirrored WHERE owner_id=$1 —
// never another owner's node), so a foreign node id degrades to the
// picker's own "no artifacts" empty state, never a leak or a 500.
func (s *Server) handleWebEditorArtefaktePicker(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.URL.Query().Get("node")
	if nodeID == "" {
		nodeID = r.URL.Query().Get("projectId")
	}
	q := r.URL.Query().Get("aq")
	var rows []components.InsertPickerRow
	if nodeID != "" {
		if arts, err := s.ListArtifacts.Execute(r.Context(), u.ID, nodeID); err == nil {
			rows = webui.BuildArtefaktInsertRows(arts, q)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = components.InsertPickerRows(rows, "editor.insertArtifact.empty").Render(r.Context(), w)
}

// handleWebEditorSeitenPicker serves GET /ui/editor/seiten (?q=): the
// "Seite verlinken ⋯" toolbar button's picker fragment, fuzzy/MRU over the
// owner's documents (Muster ⌘K-Palette, webui.BuildPaletteVM) — each row
// inserts a [[path]] wikilink. ListDocuments is owner-scoped
// (ports.DocumentStore), so a second owner's documents never appear here
// regardless of q.
func (s *Server) handleWebEditorSeitenPicker(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query().Get("q")
	docs, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	rows := webui.BuildSeitenInsertRows(docs, q)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = components.InsertPickerRows(rows, "editor.insertPage.empty").Render(r.Context(), w)
}
