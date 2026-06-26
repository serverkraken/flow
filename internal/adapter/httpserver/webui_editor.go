package httpserver

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) handleWebEditorPreview(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	bodyMD := r.FormValue("body")
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
	rendered := webui.RenderDocument(bodyMD, resolve)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.MarkdownPreview(rendered).Render(r.Context(), w)
}

func (s *Server) handleWebEditorNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.editorVM(r, u, webui.EditorVM{User: u.Username, Type: "free"})
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
	projectID := ""
	if doc.ProjectID != nil {
		projectID = *doc.ProjectID
	}
	vm, err := s.editorVM(r, u, webui.EditorVM{
		User: u.Username, ID: doc.ID, Type: string(doc.Type), ProjectID: projectID,
		Path: doc.Path, Title: doc.Title, Body: doc.Body,
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
	submitted := webui.EditorVM{
		User: u.Username, Type: r.FormValue("type"), ProjectID: r.FormValue("projectId"),
		Path: r.FormValue("path"), Title: r.FormValue("title"), Body: r.FormValue("body"),
	}
	var projectID *string
	if submitted.ProjectID != "" {
		projectID = &submitted.ProjectID
	}
	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type: domain.DocumentType(submitted.Type), ProjectID: projectID,
		Path: submitted.Path, Title: submitted.Title, Body: submitted.Body,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		s.renderEditorError(w, r, u, submitted, http.StatusBadRequest, err.Error())
	case errors.Is(err, ports.ErrDocumentExists):
		s.renderEditorError(w, r, u, submitted, http.StatusConflict, "a document with that path already exists")
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID}})
		http.Redirect(w, r, "/wissen/"+doc.ID, http.StatusSeeOther)
	}
}

func (s *Server) handleWebEditorUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	_, err := s.UpdateDocument.Execute(r.Context(), u.ID, id, usecase.UpdateDocumentInput{
		Title: r.FormValue("title"),
		Body:  r.FormValue("body"),
	})
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
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
	categoryHref, _ := wissenCategoryHrefAndLabel(doc)

	err = s.DeleteDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
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
	if s.ListProjects.Projects != nil {
		projects, err := s.ListProjects.Execute(r.Context(), u.ID)
		if err != nil {
			return webui.EditorVM{}, err
		}
		for _, p := range projects {
			vm.ProjectOptions = append(vm.ProjectOptions, webui.EditorOption{Value: p.ID, Label: p.Name})
		}
	}
	vm.PreviewHTML = s.renderEditorPreview(r, u, vm.Body)
	return vm, nil
}

func (s *Server) renderEditorPreview(r *http.Request, u domain.User, bodyMD string) template.HTML {
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
	return webui.RenderDocument(bodyMD, resolve)
}
