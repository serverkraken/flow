package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// cockpitPulse builds the subtree-filtered live activity feed for the
// cockpit's Puls section (cockpit_main.templ's cockpitPulseSection, top 8
// entries newest-first) — d.Pulse. The Kristall-era uebersichtData
// aggregator (rollup tiles, Work/Privat split, composition/chain cards) is
// gone with the tab strip (Task 7 Flatten); the pulse itself survives as the
// content column's fourth section. Owner-scoped throughout; any missing
// wiring or fetch failure degrades to an empty feed (never a 500).
func (s *Server) cockpitPulse(ctx context.Context, ownerID, nodeID string, now time.Time) []webui.ActivityRowVM {
	if s.ListActivity.Activities == nil || s.Stats.Nodes == nil {
		return nil
	}
	subtree, err := s.Stats.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil {
		slog.WarnContext(ctx, "cockpit pulse: subtree failed", "nodeID", nodeID, "err", err)
		return nil
	}
	subtreeIDs := make(map[string]bool, len(subtree))
	for _, n := range subtree {
		subtreeIDs[n.ID] = true
	}
	entries, _, err := s.ListActivity.Execute(ctx, ownerID, nil, nil, 50, 0)
	if err != nil {
		return nil
	}
	filtered := webui.FilterPulse(entries, subtreeIDs)
	if len(filtered) == 0 {
		return nil
	}
	top := filtered
	if len(top) > 8 {
		top = top[:8]
	}
	// nodeMaps resolves each pulse target's live name+kind; on failure the rows
	// would silently render every target as an orphaned (deleted-node)
	// snapshot, so log it rather than swallow.
	names, _, kinds, nerr := s.nodeMaps(ctx, ownerID)
	if nerr != nil {
		slog.WarnContext(ctx, "cockpit pulse: nodeMaps failed", "err", nerr)
	}
	return webui.BuildActivityRows(top, names, kinds, now)
}

// railContributors returns up to 4 distinct actors (human or agent) active
// anywhere in the node's subtree, freshest-first. It powers the persistent
// rail's "Beiträger" row, so it runs on nodeCockpitData's always-executed
// path — the rail reloads independently on session/node SSE events and after
// timer mutations. Owner-scoped throughout; degrades to nil (row hidden) on
// any missing wiring/error.
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
