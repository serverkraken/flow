package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type startReq struct {
	ProjectID *string    `json:"projectId"`
	Tag       string     `json:"tag"`
	Note      string     `json:"note"`
	Start     *time.Time `json:"start"`
	Stop      *time.Time `json:"stop"`
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req startReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Nachbuchen: both timestamps present → create a complete past session.
	if req.Start != nil || req.Stop != nil {
		if req.Start == nil || req.Stop == nil {
			http.Error(w, "start and stop are required together", http.StatusBadRequest)
			return
		}
		sess, err := s.AddSession.Execute(r.Context(), u.ID, req.ProjectID, *req.Start, *req.Stop, req.Tag, req.Note)
		switch {
		case errors.Is(err, domain.ErrStopBeforeStart),
			errors.Is(err, domain.ErrFutureSession),
			errors.Is(err, domain.ErrInvalidSession):
			http.Error(w, "invalid session times", http.StatusBadRequest)
			return
		case errors.Is(err, domain.ErrOverlap):
			http.Error(w, "session overlaps an existing session", http.StatusConflict)
			return
		case err != nil:
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
		writeJSON(w, http.StatusCreated, sess)
		return
	}

	// Live start (unchanged).
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
	var (
		list []domain.WorkSession
		err  error
	)
	if q := r.URL.Query().Get("until"); q != "" {
		until, perr := time.Parse(time.RFC3339, q)
		if perr != nil {
			http.Error(w, "bad until", http.StatusBadRequest)
			return
		}
		list, err = s.ListSessionsRange.Execute(r.Context(), u.ID, since, until)
	} else {
		list, err = s.ListSessions.Execute(r.Context(), u.ID, since)
	}
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

type editSessionReq struct {
	ProjectID *string    `json:"projectId"`
	Tag       string     `json:"tag"`
	Note      string     `json:"note"`
	Start     time.Time  `json:"start"`
	Stop      *time.Time `json:"stop"`
}

func (s *Server) handleEditSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req editSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Start.IsZero() {
		http.Error(w, "start required", http.StatusBadRequest)
		return
	}
	sess, err := s.EditSession.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.EditSessionInput{
		ProjectID: req.ProjectID, Tag: req.Tag, Note: req.Note, Start: req.Start, Stop: req.Stop,
	})
	switch {
	case errors.Is(err, domain.ErrStopBeforeStart):
		http.Error(w, "invalid session times", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	switch err := s.DeleteSession.Execute(r.Context(), u.ID, id); {
	case errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

// startOfDay truncates t to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
