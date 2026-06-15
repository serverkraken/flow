package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) docsListData(r *http.Request, u domain.User) (webui.DocsPageData, error) {
	list, err := s.ListDocuments.Execute(r.Context(), u.ID)
	if err != nil {
		return webui.DocsPageData{}, err
	}
	rows := make([]webui.DocRow, 0, len(list))
	for _, d := range list {
		rows = append(rows, webui.DocRow{
			ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title,
		})
	}
	return webui.DocsPageData{User: u.Username, Docs: rows}, nil
}

func (s *Server) handleWebDocsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.docsListData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocsPage(d).Render(r.Context(), w)
}

// handleWebDocsList renders just the list fragment (innerHTML swap for SSE).
func (s *Server) handleWebDocsList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.docsListData(r, u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocsFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebDocView(w http.ResponseWriter, r *http.Request) {
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
	rendered := webui.RenderMarkdown(doc.Body)
	d := webui.DocsPageData{
		User: u.Username,
		Current: &webui.DocDetail{
			ID:    doc.ID,
			Type:  string(doc.Type),
			Path:  doc.Path,
			Title: doc.Title,
			HTML:  rendered,
			Body:  doc.Body,
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocView(d).Render(r.Context(), w)
}

func (s *Server) handleWebDocNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := webui.DocsPageData{User: u.Username}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocForm(d, nil).Render(r.Context(), w)
}

func (s *Server) handleWebDocEdit(w http.ResponseWriter, r *http.Request) {
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
	d := webui.DocsPageData{User: u.Username}
	editing := &webui.DocDetail{
		ID: doc.ID, Type: string(doc.Type), Path: doc.Path, Title: doc.Title, Body: doc.Body,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocForm(d, editing).Render(r.Context(), w)
}

func (s *Server) handleWebDocCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()

	var projID *string
	if v := r.FormValue("projectId"); v != "" {
		projID = &v
	}

	// Capture submitted values for form re-population on error.
	submitted := &webui.DocFormValues{
		Type:      r.FormValue("type"),
		ProjectID: r.FormValue("projectId"),
		Path:      r.FormValue("path"),
		Title:     r.FormValue("title"),
		Body:      r.FormValue("body"),
	}

	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type:      domain.DocumentType(submitted.Type),
		ProjectID: projID,
		Path:      submitted.Path,
		Title:     submitted.Title,
		Body:      submitted.Body,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		d := webui.DocsPageData{User: u.Username, Error: err.Error(), Form: submitted}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.DocForm(d, nil).Render(r.Context(), w)
	case errors.Is(err, ports.ErrDocumentExists):
		d := webui.DocsPageData{User: u.Username, Error: "a document with that path already exists", Form: submitted}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = webui.DocForm(d, nil).Render(r.Context(), w)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID}})
		http.Redirect(w, r, "/docs/"+doc.ID, http.StatusSeeOther)
	}
}

func (s *Server) handleWebDocUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()

	// Fetch existing doc to preserve immutable tags (FIX 4: prevent tag data-loss).
	existing, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	_, err = s.UpdateDocument.Execute(r.Context(), u.ID, id, usecase.UpdateDocumentInput{
		Title: r.FormValue("title"),
		Body:  r.FormValue("body"),
		Tags:  existing.Tags,
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
	http.Redirect(w, r, "/docs/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebDocDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	err := s.DeleteDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/docs", http.StatusSeeOther)
}
