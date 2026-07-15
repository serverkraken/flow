package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	NodeID *string    `json:"projectId"`
	Tags   []string   `json:"tags"`
	Note   string     `json:"note"`
	Start  *time.Time `json:"start"`
	Stop   *time.Time `json:"stop"`
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req startReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, true) {
		return
	}

	// Nachbuchen: both timestamps present → create a complete past session.
	if req.Start != nil || req.Stop != nil {
		if req.Start == nil || req.Stop == nil {
			http.Error(w, "start and stop are required together", http.StatusBadRequest)
			return
		}
		sess, err := s.AddSession.Execute(r.Context(), u.ID, req.NodeID, *req.Start, *req.Stop, req.Tags, req.Note)
		switch {
		case errors.Is(err, domain.ErrStopBeforeStart),
			errors.Is(err, domain.ErrFutureSession),
			errors.Is(err, domain.ErrInvalidSession):
			http.Error(w, "invalid session times", http.StatusBadRequest)
			return
		case errors.Is(err, domain.ErrInvalidNode):
			http.Error(w, "worktime can only be booked to a bookable node (engagement, vorhaben or repo)", http.StatusBadRequest)
			return
		case errors.Is(err, ports.ErrNodeNotFound):
			http.Error(w, "not found", http.StatusNotFound)
			return
		case errors.Is(err, domain.ErrOverlap):
			http.Error(w, "session overlaps an existing session", http.StatusConflict)
			return
		case err != nil:
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
		writeJSON(w, http.StatusCreated, sess)
		return
	}

	// Live start.
	sess, err := s.StartSession.Execute(r.Context(), u.ID, req.NodeID, req.Tags, req.Note)
	switch {
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "worktime can only be booked to a bookable node (engagement, vorhaben or repo)", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrAlreadyRunning):
		http.Error(w, "a session is already running", http.StatusConflict)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	writeJSON(w, http.StatusCreated, sess)
}

type stopReq struct {
	NodeID *string `json:"projectId"`
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req stopReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, true) {
		return
	}
	sess, err := s.StopSession.Execute(r.Context(), u.ID, r.PathValue("id"), req.NodeID)
	switch {
	case errors.Is(err, domain.ErrProjectRequired):
		http.Error(w, "a project is required", http.StatusBadRequest)
		return
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "worktime can only be booked to a bookable node (engagement, vorhaben or repo)", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrNodeNotFound) || errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())

	// Paginated all-time mode: ?limit (and optional ?offset). Newest-first.
	if q := r.URL.Query().Get("limit"); q != "" {
		limit, err := strconv.Atoi(q)
		if err != nil || limit < 1 || limit > 200 {
			http.Error(w, "bad limit (1..200)", http.StatusBadRequest)
			return
		}
		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			if offset, err = strconv.Atoi(o); err != nil || offset < 0 {
				http.Error(w, "bad offset", http.StatusBadRequest)
				return
			}
		}
		list, total, err := s.ListSessionsPage.Execute(r.Context(), u.ID, limit, offset)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []domain.WorkSession{}
		}
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		writeJSON(w, http.StatusOK, list)
		return
	}

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

type createNodeReq struct {
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Kind               string  `json:"kind"`
	ParentID           *string `json:"parentId"`
	Color              string  `json:"color"`
	Glyph              string  `json:"glyph"`
	Icon               string  `json:"icon"`
	Description        string  `json:"description"`
	UpstreamGit        string  `json:"upstreamGit"`
	CountsTowardTarget *bool   `json:"countsTowardTarget"`
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createNodeReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Kind == "" {
		http.Error(w, "kind required", http.StatusBadRequest)
		return
	}
	// Reject a bad upstream up front so we never create a half-configured project.
	if req.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(req.UpstreamGit); !ok {
			http.Error(w, "invalid upstream git url", http.StatusBadRequest)
			return
		}
	}
	p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
		Name: req.Name, Slug: req.Slug, Color: req.Color, Glyph: req.Glyph, Icon: req.Icon,
		Kind: domain.NodeKind(req.Kind), ParentID: req.ParentID,
		CountsTowardTarget: req.CountsTowardTarget,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrNodeSlugTaken):
		http.Error(w, "a sibling node already uses this slug", http.StatusConflict)
		return
	case errors.Is(err, ports.ErrNodeNotFound): // parent referenced but absent
		http.Error(w, "parent not found", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Apply optional description/upstream (auto-syncs the remote binding).
	if req.Description != "" || req.UpstreamGit != "" {
		p, err = s.UpdateNode.Execute(r.Context(), u.ID, p.ID, usecase.UpdateNodeInput{
			Name: sp(p.Name), Slug: sp(p.Slug), Color: sp(p.Color), Glyph: sp(p.Glyph), Icon: sp(p.Icon),
			Description: sp(req.Description), UpstreamGit: sp(req.UpstreamGit), Status: nsp(p.Status),
		})
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListNodes.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	list = filterProjectsByStatus(list, r.URL.Query().Get("status"))
	if list == nil {
		list = []domain.Node{}
	}
	writeJSON(w, http.StatusOK, list)
}

// filterProjectsByStatus keeps projects whose status is in the comma-separated
// `status` query (e.g. "active,paused"). Empty query → all (backward compatible).
func filterProjectsByStatus(in []domain.Node, status string) []domain.Node {
	status = strings.TrimSpace(status)
	if status == "" {
		return in
	}
	want := map[string]bool{}
	for _, s := range strings.Split(status, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want[s] = true
		}
	}
	out := []domain.Node{}
	for _, p := range in {
		if want[string(p.Status)] {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	switch err := s.DeleteNode.Execute(r.Context(), u.ID, id); {
	case errors.Is(err, ports.ErrNodeHasChildren):
		http.Error(w, "node has children; move or remove them first", http.StatusConflict)
		return
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	p, err := s.GetNode.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updateProjReq struct {
	Name               *string `json:"name"`
	Slug               *string `json:"slug"`
	Color              *string `json:"color"`
	Glyph              *string `json:"glyph"`
	Icon               *string `json:"icon"`
	Description        *string `json:"description"`
	UpstreamGit        *string `json:"upstreamGit"`
	Status             *string `json:"status"`
	CountsTowardTarget *bool   `json:"countsTowardTarget"`
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req updateProjReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	in := usecase.UpdateNodeInput{
		Name: req.Name, Slug: req.Slug, Color: req.Color, Glyph: req.Glyph, Icon: req.Icon,
		Description: req.Description, UpstreamGit: req.UpstreamGit,
		CountsTowardTarget: req.CountsTowardTarget,
	}
	if req.Status != nil {
		st := domain.NodeStatus(*req.Status)
		in.Status = &st
	}
	p, err := s.UpdateNode.Execute(r.Context(), u.ID, r.PathValue("id"), in)
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidNode) || errors.Is(err, domain.ErrInvalidUpstream):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
	writeJSON(w, http.StatusOK, p)
}

type editSessionReq struct {
	NodeID *string    `json:"projectId"`
	Tags   *[]string  `json:"tags"`
	Note   string     `json:"note"`
	Start  time.Time  `json:"start"`
	Stop   *time.Time `json:"stop"`
}

func (s *Server) handleEditSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req editSessionReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	if req.Start.IsZero() {
		http.Error(w, "start required", http.StatusBadRequest)
		return
	}
	sess, err := s.EditSession.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.EditSessionInput{
		NodeID: req.NodeID, Tags: req.Tags, Note: req.Note, Start: req.Start, Stop: req.Stop,
	})
	switch {
	case errors.Is(err, domain.ErrStopBeforeStart):
		http.Error(w, "invalid session times", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrOverlap):
		http.Error(w, "session overlaps an existing session", http.StatusConflict)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
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
	// deleted: the session is gone — id only, no target (documented non-goal).
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

type reassignReq struct {
	IDs    []string `json:"ids"`
	NodeID string   `json:"projectId"`
}

func (s *Server) handleReassignSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req reassignReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	n, err := s.BulkAssignNode.Execute(r.Context(), u.ID, req.IDs, req.NodeID)
	switch {
	case errors.Is(err, usecase.ErrNoSessions):
		http.Error(w, "no sessions selected", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "project not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "invalid project", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}

type bulkDeleteReq struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleBulkDeleteSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req bulkDeleteReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	n, err := s.BulkDeleteSessions.Execute(r.Context(), u.ID, req.IDs)
	switch {
	case errors.Is(err, usecase.ErrNoSessions):
		http.Error(w, "no sessions selected", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID})
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

func (s *Server) handleTagTimes(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var from, to time.Time
	if q := r.URL.Query().Get("from"); q != "" {
		t, err := time.Parse(time.RFC3339, q)
		if err != nil {
			http.Error(w, "bad from (want RFC3339)", http.StatusBadRequest)
			return
		}
		from = t
	}
	if q := r.URL.Query().Get("to"); q != "" {
		t, err := time.Parse(time.RFC3339, q)
		if err != nil {
			http.Error(w, "bad to (want RFC3339)", http.StatusBadRequest)
			return
		}
		to = t
	}
	tt, err := s.TagTimeReport.Execute(r.Context(), u.ID, from, to)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tt == nil {
		tt = []domain.TagTime{}
	}
	writeJSON(w, http.StatusOK, tt)
}

// startOfDay truncates t to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
