package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// nodeCockpitData assembles the cockpit head + the active tab's panel data.
func (s *Server) nodeCockpitData(r *http.Request, u domain.User, id, activeTab string) (webui.NodeCockpit, error) {
	ctx := r.Context()
	now := s.Clock.Now()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, err
	}
	d := webui.NodeCockpit{User: u.Username, N: n, ActiveTab: webui.NormalizeTab(activeTab)}

	// Ancestor chain (leaf→root) for the breadcrumb + rate resolution.
	d.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)
	if n.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(n.Description, func(string) (string, string, bool) { return "", "", false })
	}

	// Subtree rollup (replaces the old in-process own-only sum).
	if roll, rerr := s.Stats.NodeStats(ctx, u.ID, n.ID); rerr == nil {
		d.Rollup = roll
		// Inherited rate over [node]+ancestors (leaf→root).
		chain := d.Ancestors
		if len(chain) == 0 || chain[0].ID != n.ID {
			chain = append([]domain.Node{n}, chain...)
		}
		if rate := domain.ResolveRate(chain); rate != nil {
			d.Rate = rateLabel(rate)
			d.Earnings = rate.Mul(roll.Total).String()
		}
	}

	// Timer state from the running session.
	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, gerr := s.GetRunningSession.Execute(ctx, u.ID); gerr == nil && ok {
			r2 := rs
			running = &r2
		}
	}
	nameOf := s.nodeNameLookup(ctx, u.ID)
	d.Timer = webui.NodeTimer(running, n.ID, domain.IsBookable(n.Kind), now, nameOf)

	// Active-tab data (filled by Tasks 5–8; Task 2 leaves them empty).
	return d, nil
}

// nodeNameLookup returns a closure mapping node id → name (for "running on Y").
func (s *Server) nodeNameLookup(ctx context.Context, ownerID string) func(string) string {
	all, _ := s.ListNodes.Execute(ctx, ownerID)
	m := make(map[string]string, len(all))
	for _, n := range all {
		m[n.ID] = n.Name
	}
	return func(id string) string { return m[id] }
}

// handleWebNodeView serves GET /nodes/{id}?tab= : the cockpit page.
func (s *Server) handleWebNodeView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), r.URL.Query().Get("tab"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.fillPanelData(r, u, &d)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeView(d).Render(r.Context(), w)
}

// handleWebNodeTab serves GET /nodes/{id}/tab/{name}: the tabstrip+panel fragment.
func (s *Server) handleWebNodeTab(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), r.PathValue("name"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.fillPanelData(r, u, &d) // Tasks 5–8 populate the active tab's slice
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitTabsAndPanel(d).Render(r.Context(), w)
}

// fillPanelData loads the active tab's data into d.
func (s *Server) fillPanelData(r *http.Request, u domain.User, d *webui.NodeCockpit) {
	switch d.ActiveTab {
	// case "worktime": Task 5
	// case "wissen":   Task 6
	// case "struktur": Task 7
	// case "bindings": Task 8
	}
}

// handleWebNodeHead serves GET /nodes/{id}/head : the head fragment (SSE reload).
func (s *Server) handleWebNodeHead(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), "")
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeHead(d).Render(r.Context(), w)
}

// handleWebNodeStart starts a timer pre-booked to {id}. Mirrors handleHomeStart
// but passes the node id at start (StartSession validates IsBookable -> 400).
func (s *Server) handleWebNodeStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if _, err := s.StartSession.Execute(r.Context(), u.ID, &id, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
		// already running, etc. — fall through and re-render current state.
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderNodeHead(w, r, u, id)
}

// handleWebNodeStop stops the running session and books it to {id}.
func (s *Server) handleWebNodeStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		nid := id
		if _, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &nid); err == nil {
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
		}
	}
	s.renderNodeHead(w, r, u, id)
}

// handleWebNodeSwitch stops whatever is running, then starts a timer on {id}.
func (s *Server) handleWebNodeSwitch(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		// Book the stopped session to its own node when bound; else to {id}.
		stopNode := id
		if rs.NodeID != nil {
			stopNode = *rs.NodeID
		}
		if _, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &stopNode); err != nil {
			http.Error(w, "could not switch", http.StatusBadRequest)
			return
		}
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	}
	nid := id
	if _, err := s.StartSession.Execute(r.Context(), u.ID, &nid, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderNodeHead(w, r, u, id)
}

// renderNodeHead re-renders the head fragment after a timer mutation.
func (s *Server) renderNodeHead(w http.ResponseWriter, r *http.Request, u domain.User, id string) {
	d, err := s.nodeCockpitData(r, u, id, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeHead(d).Render(r.Context(), w)
}
