package httpserver

import (
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) handleWebEditorPreview(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	bodyMD := r.FormValue("body")
	nodeID := r.FormValue("node")
	if nodeID == "" {
		nodeID = r.FormValue("projectId")
	}
	all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	resolve := func(target string) (string, string, bool) {
		if d, ok := domain.ResolveWikilink(domain.Document{OwnerID: u.ID}, target, all); ok {
			return "/wissen/" + d.ID, d.Title, true
		}
		return "", "", false
	}
	// Task 6: resolveArtifact is now built from the editor's current node —
	// the hidden "node" field (edit mode) or the "projectId" select (new
	// mode), both included on the preview POST (editor.templ's hx-include).
	// A doc/selection without a node keeps resolveArtifact nil, exactly as
	// before — every ![[slug]] embed then renders unresolved rather than a
	// 500 (Spec §13 Pflicht-Testfall covers the WITH-node case).
	resolveArtifact := s.buildEditorArtifactResolver(r, u.ID, nodeID)
	rendered, _ := webui.RenderDocument(r.Context(), bodyMD, resolve, resolveArtifact)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.MarkdownPreview(rendered).Render(r.Context(), w)
}

// buildEditorArtifactResolver builds the same nearest-ancestor-wins artifact
// resolver buildDocumentVM uses for the read view (Task 3, buildArtifactResolver
// in webui_document.go), but keyed off the EDITOR's currently selected/known
// node instead of a persisted Document.NodeID. Both editor preview call sites
// (this handler's live keyup preview, and renderEditorPreview's initial
// page-load preview) share it, as does the editor's Artefakt-Picker handler.
//
// nodeID=="" is the FREE context (free-artifacts Task 3): a new/free note has
// no node at all, so ListArtifacts.Execute(ownerID, "") resolves to that
// owner's free library alone (no ancestor chain to build). An unwired
// ListArtifacts (some test servers only wire the worktime/document usecases,
// not L6's artifact ones — same nil-guard idiom as editorVM's
// `s.ListNodes.Nodes != nil` check just above), or any ancestor/artifact
// lookup failure (including a foreign-owner nodeID, which NodeStore.Ancestors
// resolves to an empty chain rather than an error — owner-scoped, no leak),
// all degrade to a nil resolver: every embed just stays "unresolved" rather
// than a 500 for what is, worst case, a preview-only cosmetic miss.
func (s *Server) buildEditorArtifactResolver(r *http.Request, ownerID, nodeID string) webui.ArtifactResolver {
	if s.ListArtifacts.Artifacts == nil {
		return nil
	}
	var chain []domain.Node
	if nodeID != "" {
		if s.NodeAncestors.Nodes == nil {
			return nil
		}
		c, err := s.NodeAncestors.Execute(r.Context(), ownerID, nodeID)
		if err != nil {
			return nil
		}
		chain = c
	}
	arts, err := s.ListArtifacts.Execute(r.Context(), ownerID, nodeID) // nodeID=="" → free library
	if err != nil {
		return nil
	}
	return buildArtifactResolver(chain, arts)
}

func (s *Server) handleWebEditorNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.URL.Query().Get("node")
	vm, err := s.editorVM(r, u, webui.EditorVM{User: u.Username, Type: "free", NodeID: nodeID})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EditorPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebEditorEdit(w http.ResponseWriter, r *http.Request) {
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
	nodeID := ""
	if doc.NodeID != nil {
		nodeID = *doc.NodeID
	}
	vm, err := s.editorVM(r, u, webui.EditorVM{
		User: u.Username, ID: doc.ID, Type: string(doc.Type), NodeID: nodeID,
		Path: doc.Path, Title: doc.Title, TagsCSV: strings.Join(doc.Tags, " "), Body: doc.Body,
	})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EditorPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebEditorCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	tags := strings.Fields(r.FormValue("tags"))
	submitted := webui.EditorVM{
		User: u.Username, Type: r.FormValue("type"), NodeID: r.FormValue("projectId"),
		Path: r.FormValue("path"), Title: r.FormValue("title"), TagsCSV: r.FormValue("tags"),
		Body: r.FormValue("body"),
	}
	var nodeID *string
	if submitted.NodeID != "" {
		nodeID = &submitted.NodeID
	}
	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type: domain.DocumentType(submitted.Type), NodeID: nodeID,
		Path: submitted.Path, Title: submitted.Title, Tags: tags, Body: submitted.Body,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		s.renderEditorError(w, r, u, submitted, http.StatusBadRequest, err.Error())
	case errors.Is(err, ports.ErrDocumentExists):
		s.renderEditorError(w, r, u, submitted, http.StatusConflict, "a document with that path already exists")
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID, "title": doc.Title}})
		http.Redirect(w, r, "/wissen/"+doc.ID, http.StatusSeeOther)
	}
}

func (s *Server) handleWebEditorUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	tags := strings.Fields(r.FormValue("tags"))
	_, err := s.UpdateDocument.Execute(r.Context(), u.ID, id, usecase.UpdateDocumentInput{
		Title: r.FormValue("title"),
		Body:  r.FormValue("body"),
		Tags:  &tags,
	})
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id, "title": r.FormValue("title")}})
	http.Redirect(w, r, "/wissen/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebEditorDelete(w http.ResponseWriter, r *http.Request) {
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
	categoryHref := wissenShelfHref(doc)

	err = s.DeleteDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id, "title": doc.Title}})
	http.Redirect(w, r, categoryHref, http.StatusSeeOther)
}

func (s *Server) renderEditorError(w http.ResponseWriter, r *http.Request, u domain.User, vm webui.EditorVM, status int, msg string) {
	vm.User = u.Username
	vm.Err = msg
	full, err := s.editorVM(r, u, vm)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = webui.EditorPage(full).Render(r.Context(), w)
}

func (s *Server) editorVM(r *http.Request, u domain.User, vm webui.EditorVM) (webui.EditorVM, error) {
	if vm.Type == "" {
		vm.Type = "free"
	}
	vm.TypeOptions = webui.DocumentTypeOptions(vm.Type)
	if s.ListNodes.Nodes != nil {
		projects, err := s.ListNodes.Execute(r.Context(), u.ID)
		if err != nil {
			return webui.EditorVM{}, err
		}
		vm.ProjectOptions = webui.NodeSelectOptions(r.Context(), projects)
	}
	vm.PreviewHTML = s.renderEditorPreview(r, u, vm.NodeID, vm.Body)
	return vm, nil
}

func (s *Server) renderEditorPreview(r *http.Request, u domain.User, nodeID, bodyMD string) template.HTML {
	all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
	if err != nil {
		return ""
	}
	resolve := func(target string) (string, string, bool) {
		if d, ok := domain.ResolveWikilink(domain.Document{OwnerID: u.ID}, target, all); ok {
			return "/wissen/" + d.ID, d.Title, true
		}
		return "", "", false
	}
	// Task 6: same node-aware resolver as handleWebEditorPreview — see
	// buildEditorArtifactResolver's doc comment.
	resolveArtifact := s.buildEditorArtifactResolver(r, u.ID, nodeID)
	rendered, _ := webui.RenderDocument(r.Context(), bodyMD, resolve, resolveArtifact)
	return rendered
}
