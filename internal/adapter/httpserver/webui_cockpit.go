package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// nodeCockpitData assembles the cockpit rail + the active tab's panel data.
func (s *Server) nodeCockpitData(r *http.Request, u domain.User, id, activeTab string) (webui.NodeCockpit, error) {
	ctx := r.Context()
	now := s.Clock.Now()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, err
	}
	d := webui.NodeCockpit{User: u.Username, N: n, ActiveTab: webui.NormalizeTab(activeTab), Today: now.Format(dayLayout)}

	// Ancestor chain (leaf→root, self included) for the breadcrumb + rate resolution.
	d.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)
	if n.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(n.Description, func(string) (string, string, bool) { return "", "", false })
	}
	chain := d.Ancestors
	if len(chain) == 0 || chain[0].ID != n.ID {
		chain = append([]domain.Node{n}, chain...)
	}

	// Subtree rollup (replaces the old in-process own-only sum).
	if roll, rerr := s.Stats.NodeStats(ctx, u.ID, n.ID); rerr == nil {
		d.Rollup = roll
		// Inherited rate over [node]+ancestors (leaf→root).
		if rate := domain.ResolveRate(chain); rate != nil {
			d.Rate = rateLabel(rate)
			d.Earnings = rate.Mul(roll.Total).String()
		}
	}
	d.CountsWork = domain.ResolveCountsTowardTarget(chain)

	// All owner nodes, fetched once: feeds the timer's "running on Y" name
	// lookup AND the Struktur tab count below (was two separate ListNodes
	// calls before the rail needed the count too).
	all, _ := s.ListNodes.Execute(ctx, u.ID)
	names := make(map[string]string, len(all))
	childCount := 0
	for _, on := range all {
		names[on.ID] = on.Name
		if on.ParentID != nil && *on.ParentID == n.ID {
			childCount++
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
	d.Timer = webui.NodeTimer(running, n.ID, domain.IsBookable(n.Kind), now, func(id string) string { return names[id] })

	// Rail identity: logo auto-crop decision (LogoShape errors default to the
	// pre-existing hex-crop behavior — never block rendering on a logo read).
	if n.LogoRef != "" {
		d.LogoShape = "hex"
		if s.GetNodeLogo.Logos != nil {
			if logo, lerr := s.GetNodeLogo.Execute(ctx, u.ID, n.ID); lerr == nil {
				d.LogoShape = webui.LogoShape(logo.Width, logo.Height)
			}
		}
	}

	// Rail timer: today's OWN-node time (not subtree — mirrors heuteDataFor's
	// day-range source: startOfDay(now)..+1d via ListSessionsRange).
	d.TodayHere = webui.FmtDurHMExport(0)
	if s.ListSessionsRange.Sessions != nil {
		day := startOfDay(now)
		if sessions, serr := s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1)); serr == nil {
			var todaySum time.Duration
			for _, sess := range sessions {
				if sess.NodeID != nil && *sess.NodeID == n.ID {
					todaySum += sess.Elapsed(now)
				}
			}
			d.TodayHere = webui.FmtDurHMExport(todaySum)
		}
	}

	// Tab-strip counts (wissen/struktur/bindings; uebersicht/worktime carry no
	// badge). A failed count degrades to 0 rather than failing the page.
	wissenCount := 0
	if s.ListDocuments.Docs != nil {
		if docs, derr := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil); derr == nil {
			wissenCount = len(docs)
		} else {
			slog.WarnContext(ctx, "cockpit: wissen tab count failed", "nodeID", n.ID, "err", derr)
		}
	}
	bindingsCount := 0
	if s.ListNodeBindings.Bindings != nil {
		if bindings, berr := s.ListNodeBindings.ExecuteByProject(ctx, u.ID, n.ID); berr == nil {
			bindingsCount = len(bindings)
		} else {
			slog.WarnContext(ctx, "cockpit: bindings tab count failed", "nodeID", n.ID, "err", berr)
		}
	}
	d.TabCounts = map[string]int{
		"wissen":   wissenCount,
		"struktur": childCount,
		"bindings": bindingsCount,
	}

	// Rail "Beiträger" row: distinct actors (human/agent) active anywhere in
	// the subtree, max 4. Filled HERE on nodeCockpitData's always-run path —
	// not in the uebersicht panel builder — because the rail is persistent and
	// reloads independently on its own SSE events (/head) and after timer
	// mutations, none of which run fillPanelData. Filling it only in the panel
	// path would make the row appear on the uebersicht tab's first paint and
	// then vanish on the very next live reload.
	d.Contributors = s.railContributors(ctx, u.ID, n.ID)

	// Active-tab data (filled by fillPanelData).
	return d, nil
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
	case "uebersicht":
		vm, err := s.uebersichtData(r.Context(), u, d)
		if err != nil {
			d.PanelErr = err.Error()
		}
		d.Uebersicht = vm
	case "worktime":
		now := s.Clock.Now()
		since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		all, _ := s.ListSessionsRange.Execute(r.Context(), u.ID, since, now.AddDate(0, 0, 1))

		// Containment (Spec §4): an Engagement/Vorhaben cockpit lists its whole
		// SUBTREE's sessions (each row carries a node-pill — cockpit.templ);
		// a Repo cockpit lists ONLY its own (unchanged pre-Task-6 behavior).
		// allowed/names/kinds default to the own-node-only case and are widened
		// to the subtree below — same Subtree() source T5's uebersichtData uses,
		// no new port. Owner-scoped throughout (Subtree takes u.ID).
		allowed := map[string]bool{d.N.ID: true}
		names := map[string]string{d.N.ID: d.N.Name}
		kinds := map[string]domain.NodeKind{d.N.ID: d.N.Kind}
		if d.N.Kind != domain.KindRepo && s.Stats.Nodes != nil {
			if subtree, serr := s.Stats.Nodes.Subtree(r.Context(), u.ID, d.N.ID); serr == nil {
				allowed = make(map[string]bool, len(subtree))
				names = make(map[string]string, len(subtree))
				kinds = make(map[string]domain.NodeKind, len(subtree))
				for _, n := range subtree {
					allowed[n.ID] = true
					names[n.ID] = n.Name
					kinds[n.ID] = n.Kind
				}
			} else {
				slog.WarnContext(r.Context(), "cockpit worktime: subtree failed", "nodeID", d.N.ID, "err", serr)
			}
		}

		out := make([]domain.WorkSession, 0, 25)
		for i := len(all) - 1; i >= 0 && len(out) < 25; i-- { // newest first
			if all[i].NodeID != nil && allowed[*all[i].NodeID] {
				out = append(out, all[i])
			}
		}
		d.SessionRows = webui.BuildCockpitSessionRows(out, now, names, kinds)

		// Edit-mode: ?edit={sid} arrives via a row's Edit round-trip link
		// (cockpitSessionRow, cockpit.templ). Resolved against the visible `out`
		// set — sufficient since the link is only ever rendered next to a
		// visible row — and never against a session outside this owner (`all`
		// is already owner-scoped via ListSessionsRange).
		if sid := r.URL.Query().Get("edit"); sid != "" {
			for i := range out {
				if out[i].ID == sid {
					sess := out[i]
					d.EditSession = &sess
					break
				}
			}
		}
	case "wissen":
		d.Docs, d.WissenScope = s.wissenTabDocs(r, u, d.N)
	case "struktur":
		all, _ := s.ListNodes.Execute(r.Context(), u.ID)
		for _, n := range all {
			if n.ParentID != nil && *n.ParentID == d.N.ID {
				label := ""
				if roll, err := s.Stats.NodeStats(r.Context(), u.ID, n.ID); err == nil && roll.Total > 0 {
					label = webui.FmtDurHMExport(roll.Total)
				}
				d.Children = append(d.Children, webui.NodeChild{N: n, Total: label})
			}
		}
		d.MoveTargets = webui.MoveTargetsFor(all, d.N)
	case "bindings":
		d.Bindings, _ = s.ListNodeBindings.ExecuteByProject(r.Context(), u.ID, d.N.ID)
	}
}

// wissenTabDocs returns the Wissen-tab documents honouring the §4 containment
// rule: an Engagement/Vorhaben shows its whole subtree's docs by default and
// own-only when ?scope=self is set; a Repo always shows own-only. It returns the
// docs plus the effective scope ("subtree"|"self") that drives the toggle.
// Owner-scoped throughout; a Subtree/List failure degrades to own-only (never a
// 500) — mirrors uebersichtData's TopDocs source, no new port.
func (s *Server) wissenTabDocs(r *http.Request, u domain.User, n domain.Node) ([]domain.Document, string) {
	ctx := r.Context()
	self := n.Kind == domain.KindRepo || r.URL.Query().Get("scope") == "self"
	if self || s.Stats.Nodes == nil {
		docs, _ := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)
		return docs, "self"
	}
	subtree, serr := s.Stats.Nodes.Subtree(ctx, u.ID, n.ID)
	if serr != nil || len(subtree) == 0 {
		if serr != nil {
			slog.WarnContext(ctx, "cockpit wissen: subtree failed", "nodeID", n.ID, "err", serr)
		}
		docs, _ := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)
		return docs, "self"
	}
	ids := make(map[string]bool, len(subtree))
	for _, sn := range subtree {
		ids[sn.ID] = true
	}
	all, err := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
	if err != nil {
		return nil, "subtree"
	}
	out := make([]domain.Document, 0, len(all))
	for _, doc := range all {
		if doc.NodeID != nil && ids[*doc.NodeID] {
			out = append(out, doc)
		}
	}
	return out, "subtree"
}

// renderNodePanel re-renders the tab strip + one panel fragment (with an optional
// inline error). The handler returns CockpitTabsAndPanel targeting #cockpit-main
// so HTMX replaces the outer container — not #cockpit-panel (nesting bug).
func (s *Server) renderNodePanel(w http.ResponseWriter, r *http.Request, u domain.User, id, tab, errMsg string) {
	d, err := s.nodeCockpitData(r, u, id, tab)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.fillPanelData(r, u, &d)
	d.PanelErr = errMsg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitTabsAndPanel(d).Render(r.Context(), w)
}

// handleWebNodeAddSession books a manual session on {id} (Nachbuchen).
// On success it re-renders the worktime tab; on validation failure it returns
// the panel with an inline error so the user can correct the form.
func (s *Server) handleWebNodeAddSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderNodePanel(w, r, u, id, "worktime", "ungültige Zeit — HH:MM, bis > von")
		return
	}
	nid := id
	sess, err := s.AddSession.Execute(r.Context(), u.ID, &nid, start, stop,
		strings.Fields(r.FormValue("tag")), r.FormValue("note"))
	if err != nil {
		s.renderNodePanel(w, r, u, id, "worktime", "konnte nicht buchen: "+err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{
		Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
	})
	s.renderNodePanel(w, r, u, id, "worktime", "")
}

// handleWebNodeHead serves GET /nodes/{id}/head : the rail fragment (SSE
// reload target #cockpit-rail; the route path stays /head for compatibility).
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
	_ = webui.CockpitRail(d).Render(r.Context(), w)
}

// handleWebNodeStart starts a timer pre-booked to {id} (StartSession
// validates IsBookable -> 400).
func (s *Server) handleWebNodeStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if sess, err := s.StartSession.Execute(r.Context(), u.ID, &id, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
		// already running, etc. — fall through and re-render current state.
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{
			Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
		})
	}
	s.renderNodeHead(w, r, u, id)
}

// handleWebNodeStop stops the running session and books it to {id}.
func (s *Server) handleWebNodeStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		nid := id
		if sess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &nid); err == nil {
			s.Emitter.Emit(r.Context(), domain.Event{
				Type: domain.EventSessionStopped, UserID: u.ID,
				Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
			})
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
		stoppedSess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &stopNode)
		if err != nil {
			http.Error(w, "could not switch", http.StatusBadRequest)
			return
		}
		s.Emitter.Emit(r.Context(), domain.Event{
			Type: domain.EventSessionStopped, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, stoppedSess.ID, stoppedSess.NodeID),
		})
	}
	nid := id
	if sess, err := s.StartSession.Execute(r.Context(), u.ID, &nid, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{
			Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
		})
	}
	s.renderNodeHead(w, r, u, id)
}

// renderNodeHead re-renders the rail fragment after a timer mutation
// (start/stop/switch) — the rail's own hx-target is #cockpit-rail.
func (s *Server) renderNodeHead(w http.ResponseWriter, r *http.Request, u domain.User, id string) {
	d, err := s.nodeCockpitData(r, u, id, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitRail(d).Render(r.Context(), w)
}

// handleWebNodeBindRemote adds a remote binding (form field remoteSlug) to {id}.
func (s *Server) handleWebNodeBindRemote(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	slug := strings.TrimSpace(r.FormValue("remoteSlug"))
	if slug == "" {
		s.renderNodePanel(w, r, u, id, "bindings", "")
		return
	}
	key := usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: slug}
	if _, err := s.BindNode.Execute(r.Context(), u.ID, id, key); err != nil {
		msg := "konnte nicht binden"
		if errors.Is(err, usecase.ErrInvalidBindTarget) {
			msg = i18nT(r, "cockpit.bindings.remoteOnlyRepo")
		}
		s.renderNodePanel(w, r, u, id, "bindings", msg)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodePanel(w, r, u, id, "bindings", "")
}

// handleWebNodeUnbind removes a binding (form: kind + slug | machine + path).
func (s *Server) handleWebNodeUnbind(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	key := usecase.BindKey{
		Kind:       domain.BindingKind(r.FormValue("kind")),
		RemoteSlug: r.FormValue("slug"),
		MachineID:  r.FormValue("machine"),
		Path:       r.FormValue("path"),
	}
	if err := s.UnbindNode.Execute(r.Context(), u.ID, key); err != nil {
		s.renderNodePanel(w, r, u, id, "bindings", "konnte nicht lösen")
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodePanel(w, r, u, id, "bindings", "")
}
