package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// einstiegFeedRows / einstiegMarkRows are what Screen 02 shows — the store
// returns exactly these, subtree-scoped, so there is no scanning cap to
// tune and no neighbour that can crowd the register out (Soenne, 21.08.).
const (
	einstiegFeedRows = 10
	einstiegMarkRows = 5
)

// einstiegData loads the register entry point, owner-scoped, ONE query per
// sort. It deliberately does NOT call Stats.NodeStats per child — every
// per-child figure is derived from the sessions/docs already in hand.
func (s *Server) einstiegData(r *http.Request, u domain.User, id string) (webui.NodeEinstieg, error) {
	ctx := r.Context()
	now := s.Clock.Now()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeEinstieg{}, err
	}
	in := webui.EinstiegInput{N: n, Now: now, Today: now.Format(dayLayout)}
	in.SortByName = r.URL.Query().Get("sort") == "name"

	in.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)
	in.AllNodes, _ = s.ListNodes.Execute(ctx, u.ID)

	// Subtree — feeds the VM's containment filters AND the paging caps below
	// (self alone when Stats.Nodes is unavailable, never a hard failure).
	subtreeIDs := map[string]bool{n.ID: true}
	if s.Stats.Nodes != nil {
		if subtree, serr := s.Stats.Nodes.Subtree(ctx, u.ID, n.ID); serr == nil {
			in.Subtree = subtree
			subtreeIDs = make(map[string]bool, len(subtree))
			for _, sn := range subtree {
				subtreeIDs[sn.ID] = true
			}
		} else {
			slog.WarnContext(ctx, "einstieg: subtree failed", "nodeID", n.ID, "err", serr)
		}
	}

	if roll, rerr := s.Stats.NodeStats(ctx, u.ID, n.ID); rerr == nil {
		in.Rollup = roll
	} else {
		slog.WarnContext(ctx, "einstieg: node stats failed", "nodeID", n.ID, "err", rerr)
	}

	if s.ListSessionsRange.Sessions != nil {
		since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		if sessions, serr := s.ListSessionsRange.Execute(ctx, u.ID, since, now.AddDate(0, 0, 1)); serr == nil {
			in.Sessions = sessions
		} else {
			slog.WarnContext(ctx, "einstieg: sessions failed", "nodeID", n.ID, "err", serr)
		}
	}

	if s.ListDocuments.Docs != nil {
		if docs, derr := s.ListDocuments.Execute(ctx, u.ID, nil, nil); derr == nil {
			in.Docs = docs
		} else {
			slog.WarnContext(ctx, "einstieg: documents failed", "nodeID", n.ID, "err", derr)
		}
	}

	subtreeList := make([]string, 0, len(subtreeIDs))
	for sid := range subtreeIDs {
		subtreeList = append(subtreeList, sid)
	}
	if s.ListActivity.Activities != nil {
		if feed, _, aerr := s.ListActivity.ForNodes(ctx, u.ID, subtreeList, einstiegFeedRows, 0); aerr == nil {
			in.Activity = feed
		} else {
			slog.WarnContext(ctx, "einstieg: subtree activity failed", "nodeID", n.ID, "err", aerr)
		}
	}
	if s.ListNewestHighlights.Highlights != nil {
		if marks, herr := s.ListNewestHighlights.ForNodes(ctx, u.ID, subtreeList, einstiegMarkRows); herr == nil {
			in.Highlights = marks
		} else {
			slog.WarnContext(ctx, "einstieg: subtree highlights failed", "nodeID", n.ID, "err", herr)
		}
	}

	dayStart := startOfDay(now)
	if s.ListActivity.Activities != nil {
		if agents, aerr := s.ListActivity.AgentsSince(ctx, u.ID, subtreeList, dayStart); aerr == nil {
			in.AgentsToday = agents
		} else {
			slog.WarnContext(ctx, "einstieg: subtree agents failed", "nodeID", n.ID, "err", aerr)
		}
	}

	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, gerr := s.GetRunningSession.Execute(ctx, u.ID); gerr == nil && ok {
			r2 := rs
			running = &r2
			if rs.NodeID != nil {
				in.RunningNodeID = *rs.NodeID
			}
		}
	}

	// F5/I4: reuse the subtree IDs and activity already loaded above instead
	// of railContributors running its own Subtree + ListActivity(50,0) query.
	in.Contributors = railContributors(subtreeIDs, in.Activity)

	names := make(map[string]string, len(in.AllNodes))
	for _, on := range in.AllNodes {
		names[on.ID] = on.Name
	}

	// Satz und Satzquelle (Spec C.1.1/C.1.5/C.1.8) — Muster nodeCockpitData:31-44.
	chain := in.Ancestors
	if len(chain) == 0 || chain[0].ID != n.ID {
		chain = append([]domain.Node{n}, chain...)
	}
	in.Rate = domain.ResolveRate(chain) // nil = nirgends in der Kette ein Satz
	if in.Rate != nil && n.Rate == nil {
		in.RateSource = webui.RateSourceName(chain, n.ID)
	}
	in.CountsWork = domain.ResolveCountsTowardTarget(chain)
	if running != nil {
		in.RunningBase = int64(running.Elapsed(now).Seconds())
	}
	// Der Register-Text rendert ohne Wikilink- und Artefakt-Auflösung: die
	// Beschreibung eines Registers ist Prosa, keine verlinkte Karte.
	in.DescriptionHTML, _ = webui.RenderDocument(ctx, n.Description,
		func(string) (string, string, bool) { return "", "", false },
		nil)

	// Rail timer's own-node "today" figure — derived from in.Sessions
	// (already loaded above, since 2000-01-01) instead of a second
	// ListSessionsRange(day..day+1) query (F5/I4: same [day, day+1) window
	// ListSessionsRange itself filters on, just applied in-process).
	dayEnd := dayStart.AddDate(0, 0, 1)
	var todaySum time.Duration
	for _, sess := range in.Sessions {
		if sess.NodeID == nil || *sess.NodeID != n.ID {
			continue
		}
		if sess.Start.Before(dayStart) || !sess.Start.Before(dayEnd) {
			continue
		}
		todaySum += sess.Elapsed(now)
	}
	in.TodayHere = webui.FmtDurHMExport(todaySum)

	return webui.BuildNodeEinstieg(ctx, in), nil
}

// railContributors returns up to 4 distinct actors (human or agent) active
// anywhere in the node's subtree, freshest-first. It powers the Kasten
// column's "Beiträger" row. F5/I4: reuses the subtree IDs and activity
// einstiegData already loaded (own Subtree()/ListActivity() query removed —
// Faktor 3 on the request's most expensive query is now Faktor 1).
func railContributors(subtreeIDs map[string]bool, activity []domain.ActivityEntry) []string {
	var out []string
	seen := make(map[string]bool, 4)
	for _, e := range webui.FilterPulse(activity, subtreeIDs) {
		if seen[e.ActorRef] {
			continue
		}
		seen[e.ActorRef] = true
		out = append(out, e.ActorRef)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// handleWebNodeLese serves GET /nodes/{id}/lese : the reading-column SSE
// fragment (#einstieg-lese).
func (s *Server) handleWebNodeLese(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.einstiegData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstiegLese(d).Render(r.Context(), w)
}
