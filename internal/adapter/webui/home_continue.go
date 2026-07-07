package webui

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// RecentNode is one "Weiterarbeiten" row on the Schreibtisch: a bookable node
// the owner recently worked on, derived purely from WorkSessions — the same
// MRU projection the ⌘K-Palette already uses (webui_palette.go/
// BuildPaletteVM's recentNodeIDs), just distilled into display fields
// instead of a ranking score. No new schema.
type RecentNode struct {
	ID, Name, FullPath, Tone, Initials string
	LogoRef                            string // domain.Node.LogoRef, "" = no logo (NodeAvatar)
	ValueStr, LabelKey                 string
}

// BuildRecentNodes returns up to n distinct bookable nodes, most-recently-
// touched first (a running session's node sorts first — it is always the
// latest-started session). now drives both ValueStr branches: the running
// node gets its live elapsed duration (FmtVerbose, home.runningNow); a
// stopped node gets the app-wide relative-time convention (FmtRelTime,
// home.lastActive) already used by the Puls feed and the Wissen "Zuletzt
// aktualisiert" list — reusing it here keeps every relative timestamp on the
// Schreibtisch consistent instead of inventing a second "gestern"-style
// formatter.
func BuildRecentNodes(sessions []domain.WorkSession, nodes []domain.Node, now time.Time, n int) []RecentNode {
	byID := make(map[string]domain.Node, len(nodes))
	for _, nd := range nodes {
		byID[nd.ID] = nd
	}

	sorted := append([]domain.WorkSession(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start.After(sorted[j].Start) })

	var out []RecentNode
	seen := make(map[string]bool, n)
	for _, ws := range sorted {
		if ws.NodeID == nil || seen[*ws.NodeID] {
			continue
		}
		nd, ok := byID[*ws.NodeID]
		if !ok || !domain.IsBookable(nd.Kind) {
			continue
		}
		seen[*ws.NodeID] = true

		row := RecentNode{
			ID:       nd.ID,
			Name:     ShortName(nd.Name),
			FullPath: nd.Name,
			Tone:     AvatarTone(nd.Name),
			Initials: Initials(nd.Name),
			LogoRef:  nd.LogoRef,
		}
		if ws.Running() {
			row.ValueStr = FmtVerbose(ws.Elapsed(now))
			row.LabelKey = "home.runningNow"
		} else {
			row.ValueStr = FmtRelTime(ws.Start, now)
			row.LabelKey = "home.lastActive"
		}
		out = append(out, row)
		if len(out) == n {
			break
		}
	}
	return out
}
