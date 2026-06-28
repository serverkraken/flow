package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type putActiveReq struct {
	Remote  string   `json:"remote"`
	Machine string   `json:"machine"`
	Path    string   `json:"path"`
	Node    string   `json:"node"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags"`
}

func (s *Server) handlePutContextActive(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req putActiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, updated, err := s.SetActiveContext.Execute(r.Context(), u.ID,
		usecase.ContextResolveInput{RemoteSlug: req.Remote, MachineID: req.Machine, Cwd: req.Path, NodeOverride: req.Node},
		req.Title, req.Body, req.Tags)
	switch {
	case errors.Is(err, usecase.ErrContextUnresolved):
		http.Error(w, "repo not bound", http.StatusConflict)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "updatedAt": updated})
	}
}

func (s *Server) handleGetContext(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	in := usecase.ContextResolveInput{
		RemoteSlug:   strings.TrimSpace(q.Get("remote")),
		MachineID:    strings.TrimSpace(q.Get("machine")),
		Cwd:          strings.TrimSpace(q.Get("path")),
		NodeOverride: strings.TrimSpace(q.Get("node")),
	}
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 6000
	}
	if v := strings.TrimSpace(q.Get("cap")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			budget = n
		}
	}
	cc, err := s.ComposeContext.Execute(r.Context(), u.ID, in, budget)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cc)
}
