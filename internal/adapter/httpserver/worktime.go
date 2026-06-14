package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type startReq struct {
	ProjectID *string `json:"projectId"`
	Tag       string  `json:"tag"`
	Note      string  `json:"note"`
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req startReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess, err := s.StartSession.Execute(r.Context(), u.ID, req.ProjectID, req.Tag, req.Note)
	if errors.Is(err, domain.ErrAlreadyRunning) {
		http.Error(w, "a session is already running", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusCreated, sess)
}

type stopReq struct {
	ProjectID *string `json:"projectId"`
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req stopReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess, err := s.StopSession.Execute(r.Context(), u.ID, r.PathValue("id"), req.ProjectID)
	switch {
	case errors.Is(err, domain.ErrProjectRequired):
		http.Error(w, "a project is required", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrProjectNotFound) || errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	since := startOfDay(s.Clock.Now())
	if q := r.URL.Query().Get("since"); q != "" {
		if t, err := time.Parse(time.RFC3339, q); err == nil {
			since = t
		}
	}
	list, err := s.ListSessions.Execute(r.Context(), u.ID, since)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.WorkSession{}
	}
	writeJSON(w, http.StatusOK, list)
}

type createProjReq struct {
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Color string `json:"color"`
	Glyph string `json:"glyph"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createProjReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	p, err := s.CreateProject.Execute(r.Context(), u.ID, req.Name, req.Slug, req.Color, req.Glyph)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListProjects.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Project{}
	}
	writeJSON(w, http.StatusOK, list)
}

// startOfDay truncates t to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
