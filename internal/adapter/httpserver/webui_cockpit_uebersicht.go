package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// uebersichtData builds the "uebersicht" tab's kind-differentiated feed:
// rollup tiles, the Work/Privat split, the composition (Engagement/Vorhaben)
// or chain (Repo) card (Containment rule, spec §4), the subtree-filtered
// live pulse, and the recently-changed-knowledge card. (The rail's
// "Beiträger" row is filled separately in nodeCockpitData via
// railContributors, because the rail reloads independently of this panel.)
//
// Every I/O call degrades gracefully (missing usecase wiring or a failed
// fetch just leaves the corresponding VM field empty) — a partial feed is
// far better than a broken cockpit page. Failures are logged (slog.Warn) so a
// silently-blank feed is still distinguishable from a genuinely empty node in
// monitoring; the error return is reserved for a future hard-failure path.
func (s *Server) uebersichtData(ctx context.Context, u domain.User, d *webui.NodeCockpit) (webui.UebersichtVM, error) {
	now := s.Clock.Now()
	vm := webui.UebersichtVM{Kind: d.N.Kind}

	// Self-normalizing ancestor chain (leaf→root, self at [0]) — mirrors
	// nodeCockpitData's own defensive walk, needed here to resolve the
	// inherited rate for the Earnings tile.
	chain := d.Ancestors
	if len(chain) == 0 || chain[0].ID != d.N.ID {
		chain = append([]domain.Node{d.N}, chain...)
	}

	tiles := webui.BuildUebersichtTiles(d.Rollup, domain.ResolveRate(chain))
	vm.TotalStr, vm.WeekStr, vm.WeekDelta, vm.MonthStr, vm.Earnings = tiles.TotalStr, tiles.WeekStr, tiles.WeekDelta, tiles.MonthStr, tiles.Earnings
	vm.WorkPct, vm.HasSplit, vm.WorkWeekStr, vm.PrivatWeekStr, vm.WorkMonthStr = webui.BuildSplit(d.Rollup)

	// One Subtree() query serves the composition/chain stats loop below, the
	// pulse filter, AND the docs filter — no extra per-card query.
	var subtree []domain.Node
	if s.Stats.Nodes != nil {
		var serr error
		if subtree, serr = s.Stats.Nodes.Subtree(ctx, u.ID, d.N.ID); serr != nil {
			// The node is known-present (GetNode already succeeded upstream), so
			// a Subtree failure here cascades to blank pulse/composition/docs —
			// log it so it's not mistaken for a genuinely empty node.
			slog.WarnContext(ctx, "cockpit uebersicht: subtree failed", "nodeID", d.N.ID, "err", serr)
		}
	}
	subtreeIDs := make(map[string]bool, len(subtree))
	subtreeParents := make(map[string]string, len(subtree))
	for _, n := range subtree {
		subtreeIDs[n.ID] = true
		if n.ParentID != nil {
			subtreeParents[n.ID] = *n.ParentID
		}
	}

	var filtered []domain.ActivityEntry
	if s.ListActivity.Activities != nil {
		if entries, _, err := s.ListActivity.Execute(ctx, u.ID, nil, nil, 50, 0); err == nil {
			filtered = webui.FilterPulse(entries, subtreeIDs)
		}
	}

	if d.N.Kind == domain.KindRepo {
		vm.Chain = s.chainRows(ctx, u.ID, d, chain)
	} else {
		vm.Comp = s.compRows(ctx, u.ID, d, subtree, subtreeParents, filtered, now)
	}

	// Pulse card: top 8 of the subtree-filtered feed.
	if len(filtered) > 0 {
		top := filtered
		if len(top) > 8 {
			top = top[:8]
		}
		// nodeMaps resolves each pulse target's live name+kind; on failure the
		// rows would silently render every target as an orphaned (deleted-node)
		// snapshot, so log it rather than swallow.
		names, _, kinds, nerr := s.nodeMaps(ctx, u.ID)
		if nerr != nil {
			slog.WarnContext(ctx, "cockpit uebersicht: nodeMaps for pulse failed", "err", nerr)
		}
		vm.Pulse = webui.BuildActivityRows(top, names, kinds, now)
	}

	// Knowledge card: all owner docs filtered to the subtree, top 3 by
	// UpdatedAt desc.
	if s.ListDocuments.Docs != nil {
		if docs, err := s.ListDocuments.Execute(ctx, u.ID, nil, nil); err == nil {
			vm.Docs, vm.DocsTotal = webui.TopDocs(docs, subtreeIDs, now)
		}
	}

	return vm, nil
}

// chainRows builds the Repo "flows upward" chain: this node, its ancestors
// (leaf→root), and the owner-wide Σ row. Guarded on Stats.Nodes so an unwired
// StatsComputer degrades to an all-zero chain instead of panicking inside
// NodeStats (which dereferences its NodeStore) — the same graceful-degrade
// contract the rest of uebersichtData honors.
func (s *Server) chainRows(ctx context.Context, ownerID string, d *webui.NodeCockpit, chain []domain.Node) []webui.ChainRow {
	statsByID := map[string]domain.NodeRollup{d.N.ID: d.Rollup}
	var ownerTotal time.Duration
	if s.Stats.Nodes != nil {
		for _, a := range chain[1:] {
			if r, err := s.Stats.NodeStats(ctx, ownerID, a.ID); err == nil {
				statsByID[a.ID] = r
			}
		}
		// Owner total = Σ over the owner's ROOT engagements (their subtrees are
		// disjoint, so no double-count). Archived engagements are excluded to
		// match the nav tree's active+paused visibility (webui_nav.go) — a
		// long-done archived engagement must not silently inflate the
		// denominator and shrink every ChainRow.Pct. EXCEPTION: the viewed
		// chain's OWN root is always counted even when archived (a repo under an
		// archived engagement, reachable by direct URL, would otherwise have its
		// own hours excluded from the denominator while its rows still show them
		// — making every Pct incoherent).
		viewedRootID := chain[len(chain)-1].ID
		if s.ListNodes.Nodes != nil {
			if all, err := s.ListNodes.Execute(ctx, ownerID); err == nil {
				for _, n := range all {
					if n.ParentID == nil && n.Kind == domain.KindEngagement &&
						(n.Status != domain.NodeArchived || n.ID == viewedRootID) {
						if r, serr := s.Stats.NodeStats(ctx, ownerID, n.ID); serr == nil {
							ownerTotal += r.Total
						}
					}
				}
			} else {
				slog.WarnContext(ctx, "cockpit uebersicht: owner-total ListNodes failed", "err", err)
			}
		}
	}
	return webui.BuildChain(d.N, chain[1:], statsByID, ownerTotal)
}

// compRows builds the Engagement/Vorhaben composition: one row per direct
// child with its subtree share, live dot, and last-activity time. The child
// set comes from the already-fetched subtree (empty when Stats is unwired, so
// the per-child NodeStats loop never runs — no panic).
func (s *Server) compRows(ctx context.Context, ownerID string, d *webui.NodeCockpit, subtree []domain.Node, subtreeParents map[string]string, filtered []domain.ActivityEntry, now time.Time) []webui.CompRow {
	var children []domain.Node
	for _, n := range subtree {
		if n.ParentID != nil && *n.ParentID == d.N.ID {
			children = append(children, n)
		}
	}
	statsByID := make(map[string]domain.NodeRollup, len(children))
	for _, c := range children {
		if r, err := s.Stats.NodeStats(ctx, ownerID, c.ID); err == nil {
			statsByID[c.ID] = r
		}
	}
	runningNodeID := ""
	switch d.Timer.State {
	case webui.TimerHere:
		runningNodeID = d.N.ID
	case webui.TimerOtherBound:
		runningNodeID = d.Timer.OtherID
	}
	return webui.BuildComp(children, statsByID, runningNodeID, subtreeParents, filtered, d.Rollup.Total, now)
}

// railContributors returns up to 4 distinct actors (human or agent) active
// anywhere in the node's subtree, freshest-first. It powers the persistent
// rail's "Beiträger" row, so it runs on nodeCockpitData's always-executed
// path — the rail reloads independently on session/node SSE events and after
// timer mutations, none of which run the uebersicht panel builder. Owner-
// scoped throughout; degrades to nil (row hidden) on any missing wiring/error.
func (s *Server) railContributors(ctx context.Context, ownerID, nodeID string) []string {
	if s.ListActivity.Activities == nil || s.Stats.Nodes == nil {
		return nil
	}
	subtree, err := s.Stats.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil || len(subtree) == 0 {
		return nil
	}
	ids := make(map[string]bool, len(subtree))
	for _, n := range subtree {
		ids[n.ID] = true
	}
	entries, _, err := s.ListActivity.Execute(ctx, ownerID, nil, nil, 50, 0)
	if err != nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool, 4)
	for _, e := range webui.FilterPulse(entries, ids) {
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
