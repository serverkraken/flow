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

// nodeCockpitData is the cockpit's single-pass builder (Task 7 Flatten): it
// fills EVERYTHING the flat page needs in one call — head/spine data, the
// content column's Enthält/Wissen/Buchungen/Puls sections, and the rail's
// Kette/Bindings blocks — because CockpitMain now shows every section always
// (no more per-tab partial fill). ?scope= (Wissen) and ?edit={sid} (Buchungen)
// are read directly off the request so both the full page and the /main
// fragment honor them identically. Owner-scoped throughout (every store call
// carries u.ID).
func (s *Server) nodeCockpitData(r *http.Request, u domain.User, id string) (webui.NodeCockpit, error) {
	ctx := r.Context()
	now := s.Clock.Now()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, err
	}
	d := webui.NodeCockpit{User: u.Username, N: n, Today: now.Format(dayLayout)}

	// Ancestor chain (leaf→root, self included) for the spine crumbs + rate
	// resolution + the rail's Kette block.
	d.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)
	if n.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(n.Description, func(string) (string, string, bool) { return "", "", false })
	}
	chain := d.Ancestors
	if len(chain) == 0 || chain[0].ID != n.ID {
		chain = append([]domain.Node{n}, chain...)
	}

	// Subtree rollup + inherited rate.
	if roll, rerr := s.Stats.NodeStats(ctx, u.ID, n.ID); rerr == nil {
		d.Rollup = roll
		if rate := domain.ResolveRate(chain); rate != nil {
			d.Rate = rateLabel(rate)
			d.Earnings = rate.Mul(roll.Total).String()
		}
	}
	d.CountsWork = domain.ResolveCountsTowardTarget(chain)

	// All owner nodes, fetched once: feeds the timer's "running on Y" name
	// lookup AND the Enthält section's direct-children list below.
	all, _ := s.ListNodes.Execute(ctx, u.ID)
	names := make(map[string]string, len(all))
	for _, on := range all {
		names[on.ID] = on.Name
	}
	for _, on := range all {
		if on.ParentID != nil && *on.ParentID == n.ID {
			label := ""
			if roll, serr := s.Stats.NodeStats(ctx, u.ID, on.ID); serr == nil && roll.Total > 0 {
				label = webui.FmtDurHMExport(roll.Total)
			}
			d.Children = append(d.Children, webui.NodeChild{N: on, Total: label})
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

	// Spine identity: logo auto-crop decision (LogoShape errors default to the
	// pre-existing hex-crop behavior — never block rendering on a logo read).
	if n.LogoRef != "" {
		d.LogoShape = "hex"
		if s.GetNodeLogo.Logos != nil {
			if logo, lerr := s.GetNodeLogo.Execute(ctx, u.ID, n.ID); lerr == nil {
				d.LogoShape = webui.LogoShape(logo.Width, logo.Height)
			}
		}
	}

	// instr-band: today's OWN-node time (not subtree — mirrors heuteDataFor's
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

	// Chain stats + root: one NodeRollup per node in `chain` (self+ancestors),
	// feeding the rail's Kette block (ChainRows) and the instr-band's third
	// stats segment (the root engagement's whole-chain total).
	d.ChainStats = map[string]domain.NodeRollup{n.ID: d.Rollup}
	for _, a := range chain {
		if a.ID == n.ID {
			continue
		}
		if roll, serr := s.Stats.NodeStats(ctx, u.ID, a.ID); serr == nil {
			d.ChainStats[a.ID] = roll
		}
	}
	root := chain[len(chain)-1]
	d.ChainRootName = root.Name
	d.ChainRootTotal = webui.FmtDurHMExport(d.ChainStats[root.ID].Total)

	// Rail "Beiträger" row: distinct actors (human/agent) active anywhere in
	// the subtree, max 4.
	d.Contributors = s.railContributors(ctx, u.ID, n.ID)

	// Rail Bindings block.
	if s.ListNodeBindings.Bindings != nil {
		if bindings, berr := s.ListNodeBindings.ExecuteByProject(ctx, u.ID, n.ID); berr == nil {
			d.Bindings = bindings
		} else {
			slog.WarnContext(ctx, "cockpit: bindings failed", "nodeID", n.ID, "err", berr)
		}
	}

	// Wissen section: containment-aware (§4), honors ?scope=.
	docs, scope := s.wissenTabDocs(r, u, n)
	d.WissenScope = scope
	d.WissenRows = webui.BuildWissenRows(docs, now)

	// Buchungen section: bookable kinds only, containment-aware (§4), honors
	// ?edit={sid} for the pre-opened edit dialog.
	if domain.IsBookable(n.Kind) {
		s.fillSessionRows(r, u, n, now, &d)
	}

	// Puls section: subtree-filtered live activity feed, top 8.
	d.Pulse = s.cockpitPulse(ctx, u.ID, n.ID, now)

	return d, nil
}

// fillSessionRows loads the Buchungen section's session rows (containment:
// an Engagement/Vorhaben lists its whole SUBTREE, a Repo lists only its own —
// Spec §4) and resolves ?edit={sid} against the visible set into d.EditSession.
// Degrades to an empty list when ListSessionsRange is unwired — the Buchungen
// section is always-run now for bookable nodes (Task 7 Flatten), so a
// Sessions-less test/dev wiring must not panic.
func (s *Server) fillSessionRows(r *http.Request, u domain.User, n domain.Node, now time.Time, d *webui.NodeCockpit) {
	if s.ListSessionsRange.Sessions == nil {
		return
	}
	ctx := r.Context()
	since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	all, _ := s.ListSessionsRange.Execute(ctx, u.ID, since, now.AddDate(0, 0, 1))

	allowed := map[string]bool{n.ID: true}
	names := map[string]string{n.ID: n.Name}
	kinds := map[string]domain.NodeKind{n.ID: n.Kind}
	if n.Kind != domain.KindRepo && s.Stats.Nodes != nil {
		if subtree, serr := s.Stats.Nodes.Subtree(ctx, u.ID, n.ID); serr == nil {
			allowed = make(map[string]bool, len(subtree))
			names = make(map[string]string, len(subtree))
			kinds = make(map[string]domain.NodeKind, len(subtree))
			for _, sn := range subtree {
				allowed[sn.ID] = true
				names[sn.ID] = sn.Name
				kinds[sn.ID] = sn.Kind
			}
		} else {
			slog.WarnContext(ctx, "cockpit buchungen: subtree failed", "nodeID", n.ID, "err", serr)
		}
	}

	out := make([]domain.WorkSession, 0, 25)
	for i := len(all) - 1; i >= 0 && len(out) < 25; i-- { // newest first
		if all[i].NodeID != nil && allowed[*all[i].NodeID] {
			out = append(out, all[i])
		}
	}
	d.SessionRows = webui.BuildCockpitSessionRows(out, now, names, kinds)

	if sid := r.URL.Query().Get("edit"); sid != "" {
		for i := range out {
			if out[i].ID == sid {
				sess := out[i]
				d.EditSession = &sess
				break
			}
		}
	}
}

// wissenTabDocs returns the Wissen section's documents honouring the §4
// containment rule: an Engagement/Vorhaben shows its whole subtree's docs by
// default and own-only when ?scope=self is set; a Repo always shows own-only.
// It returns the docs plus the effective scope ("subtree"|"self") that drives
// the toggle. Owner-scoped throughout; a Subtree/List failure — or an unwired
// ListDocuments usecase — degrades to an empty own-only list (never a 500 or
// a nil-pointer panic; the Wissen section is always-run now, Task 7 Flatten,
// so every cockpit render must survive a Docs-less test/dev wiring).
func (s *Server) wissenTabDocs(r *http.Request, u domain.User, n domain.Node) ([]domain.Document, string) {
	ctx := r.Context()
	if s.ListDocuments.Docs == nil {
		return nil, "self"
	}
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
		slog.WarnContext(ctx, "cockpit wissen: list docs failed", "nodeID", n.ID, "err", err)
		docs, _ := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)
		return docs, "self"
	}
	out := make([]domain.Document, 0, len(all))
	for _, doc := range all {
		if doc.NodeID != nil && ids[*doc.NodeID] {
			out = append(out, doc)
		}
	}
	return out, "subtree"
}

// handleWebNodeView serves GET /nodes/{id}: the flat cockpit page.
func (s *Server) handleWebNodeView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeView(d).Render(r.Context(), w)
}

// handleWebNodeHead serves GET /nodes/{id}/head : the #cockpit-head fragment
// (spine only — reload target on sse:node.updated/node.moved).
func (s *Server) handleWebNodeHead(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitHead(d).Render(r.Context(), w)
}

// handleWebNodeMain serves GET /nodes/{id}/main : the #cockpit-main fragment
// (instr-band + Enthält/Wissen/Buchungen/Puls — the reload target for every
// session/document/node/activity mutation).
func (s *Server) handleWebNodeMain(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitMain(d).Render(r.Context(), w)
}

// handleWebNodeRail serves GET /nodes/{id}/rail : the #cockpit-rail fragment
// (Kette + Bindings — reload target for session/bind mutations).
func (s *Server) handleWebNodeRail(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitRailBlocks(d).Render(r.Context(), w)
}

// renderCockpitMain re-renders #cockpit-main with an optional inline error —
// the target for Start/Stop/Switch (the instr-band lives in main now),
// Nachbuchen, and session Edit/Delete.
func (s *Server) renderCockpitMain(w http.ResponseWriter, r *http.Request, u domain.User, id, errMsg string) {
	d, err := s.nodeCockpitData(r, u, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	d.PanelErr = errMsg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitMain(d).Render(r.Context(), w)
}

// renderNodeRail re-renders #cockpit-rail with an optional inline error — the
// target for bind/unbind mutations.
func (s *Server) renderNodeRail(w http.ResponseWriter, r *http.Request, u domain.User, id, errMsg string) {
	d, err := s.nodeCockpitData(r, u, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	d.PanelErr = errMsg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitRailBlocks(d).Render(r.Context(), w)
}

// handleWebNodeAddSession books a manual session on {id} (Nachbuchen).
// On success it re-renders #cockpit-main; on validation failure it returns
// the same fragment with an inline error so the user can correct the form.
func (s *Server) handleWebNodeAddSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderCockpitMain(w, r, u, id, "ungültige Zeit — HH:MM, bis > von")
		return
	}
	nid := id
	sess, err := s.AddSession.Execute(r.Context(), u.ID, &nid, start, stop,
		strings.Fields(r.FormValue("tag")), r.FormValue("note"))
	if err != nil {
		s.renderCockpitMain(w, r, u, id, "konnte nicht buchen: "+err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{
		Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
	})
	s.renderCockpitMain(w, r, u, id, "")
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
	s.renderCockpitMain(w, r, u, id, "")
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
	s.renderCockpitMain(w, r, u, id, "")
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
	s.renderCockpitMain(w, r, u, id, "")
}

// handleWebNodeBindRemote adds a remote binding (form field remoteSlug) to {id}.
func (s *Server) handleWebNodeBindRemote(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	slug := strings.TrimSpace(r.FormValue("remoteSlug"))
	if slug == "" {
		s.renderNodeRail(w, r, u, id, "")
		return
	}
	key := usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: slug}
	if _, err := s.BindNode.Execute(r.Context(), u.ID, id, key); err != nil {
		msg := "konnte nicht binden"
		if errors.Is(err, usecase.ErrInvalidBindTarget) {
			msg = i18nT(r, "cockpit.bindings.remoteOnlyRepo")
		}
		s.renderNodeRail(w, r, u, id, msg)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodeRail(w, r, u, id, "")
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
		s.renderNodeRail(w, r, u, id, "konnte nicht lösen")
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodeRail(w, r, u, id, "")
}
