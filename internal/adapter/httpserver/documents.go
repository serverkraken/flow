package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type createDocReq struct {
	Type   string   `json:"type"`
	NodeID *string  `json:"projectId"`
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags"`
}

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createDocReq
	if !decodeJSONBody(w, r, &req, maxDocumentJSONBodyBytes, false) {
		return
	}
	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type:   domain.DocumentType(req.Type),
		NodeID: req.NodeID,
		Path:   req.Path,
		Title:  req.Title,
		Body:   req.Body,
		Tags:   req.Tags,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		http.Error(w, "invalid document", http.StatusBadRequest)
	case errors.Is(err, ports.ErrDocumentExists):
		http.Error(w, "path already exists", http.StatusConflict)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "project not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		data := map[string]any{"id": doc.ID, "title": doc.Title}
		if doc.NodeID != nil {
			data["node"] = *doc.NodeID
		}
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: data})
		writeJSON(w, http.StatusCreated, doc)
	}
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags := r.URL.Query()["tag"]
	var nodeID *string
	if v := strings.TrimSpace(r.URL.Query().Get("projectId")); v != "" {
		nodeID = &v
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nodeID, tags)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if hits == nil {
			hits = []domain.SearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
		return
	}
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, nodeID, tags)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Document{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, doc)
	}
}

type updateDocReq struct {
	Title string    `json:"title"`
	Body  string    `json:"body"`
	Tags  *[]string `json:"tags"`
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req updateDocReq
	if !decodeJSONBody(w, r, &req, maxDocumentJSONBodyBytes, false) {
		return
	}
	doc, err := s.UpdateDocument.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.UpdateDocumentInput{
		Title: req.Title,
		Body:  req.Body,
		Tags:  req.Tags,
	})
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": doc.ID, "title": doc.Title}})
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	// Pre-fetch so the label snapshot survives deletion.
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if err != nil {
		slog.WarnContext(r.Context(), "delete document: pre-fetch for activity label failed", "id", id, "err", err)
	}
	err = s.DeleteDocument.Execute(r.Context(), u.ID, id)
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id, "title": doc.Title}})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var scope domain.TagScope
	if t := strings.TrimSpace(r.URL.Query().Get("type")); t != "" {
		tt := domain.TaggableType(t)
		scope.Type = &tt
	}
	tags, err := s.ListTags.Execute(r.Context(), u.ID, scope)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.TagCount{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleDocumentBacklinks(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		if refs == nil {
			refs = []domain.BacklinkRef{}
		}
		writeJSON(w, http.StatusOK, refs)
	}
}

type importDocReq struct {
	Type   string     `json:"type"`
	Path   string     `json:"path"`
	Title  string     `json:"title"`
	Body   string     `json:"body"`
	Date   *time.Time `json:"date"`
	NodeID *string    `json:"projectId"`
	Tags   []string   `json:"tags"`
}

func (s *Server) handleImportDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req importDocReq
	if !decodeJSONBody(w, r, &req, maxDocumentJSONBodyBytes, false) {
		return
	}
	doc, err := s.ImportDocument.Execute(r.Context(), u.ID, usecase.ImportDocumentInput{
		Type: domain.DocumentType(req.Type), Path: req.Path, Title: req.Title,
		Body: req.Body, Date: req.Date, NodeID: req.NodeID, Tags: req.Tags,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		http.Error(w, "invalid document", http.StatusBadRequest)
	case errors.Is(err, ports.ErrDocumentExists):
		http.Error(w, "path already exists", http.StatusConflict)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "project not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		data := map[string]any{"id": doc.ID, "title": doc.Title}
		if doc.NodeID != nil {
			data["node"] = *doc.NodeID
		}
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: data})
		writeJSON(w, http.StatusCreated, doc)
	}
}

type upsertByPathReq struct {
	Type     string   `json:"type"`
	NodeID   *string  `json:"projectId,omitempty"`
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags,omitempty"`
	Pinned   bool     `json:"pinned"`
	Archived bool     `json:"archived"`
}

func (s *Server) handleUpsertByPath(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req upsertByPathReq
	if !decodeJSONBody(w, r, &req, maxDocumentJSONBodyBytes, false) {
		return
	}
	id, updated, err := s.UpsertDocumentByPath.Execute(r.Context(), u.ID, usecase.UpsertByPathInput{
		Type: domain.DocumentType(req.Type), NodeID: req.NodeID, Path: req.Path,
		Title: req.Title, Body: req.Body, Tags: req.Tags, Pinned: req.Pinned, Archived: req.Archived,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidDocument) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ports.ErrNodeNotFound) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id, "title": req.Title}})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "updatedAt": updated})
}

type archiveReq struct {
	Archived bool `json:"archived"`
}

func (s *Server) handleArchiveDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req archiveReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	id := r.PathValue("id")
	switch err := s.SetArchived.Execute(r.Context(), u.ID, id, req.Archived); {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}

type pinReq struct {
	Pinned bool `json:"pinned"`
}

type contextModeReq struct {
	Mode string `json:"mode"`
}

func (s *Server) handleSetContextMode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req contextModeReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	id := r.PathValue("id")
	switch err := s.SetContextMode.Execute(r.Context(), u.ID, id, domain.ContextMode(req.Mode)); {
	case errors.Is(err, domain.ErrInvalidDocument):
		http.Error(w, "bad mode", http.StatusBadRequest)
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleListArchived(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListArchived.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Document{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handlePinDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req pinReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	id := r.PathValue("id")
	switch err := s.SetPinned.Execute(r.Context(), u.ID, id, req.Pinned); {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}
