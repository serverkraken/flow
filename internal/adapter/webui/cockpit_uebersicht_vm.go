package webui

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// UebersichtVM is the overview landing's card data (kind-differentiated per
// the Containment rule, spec §4): Engagement/Vorhaben carry Comp (direct
// children), Repo carries Chain (this → ancestors → owner total). Both carry
// the rollup tiles, the Work/Privat split, the subtree-filtered pulse, and
// the recently-changed-knowledge card.
type UebersichtVM struct {
	Kind domain.NodeKind

	// rollup tiles (BuildUebersichtTiles)
	TotalStr, WeekStr, WeekDelta, MonthStr, Earnings string

	// work/privat split, week window (BuildSplit). HasSplit=false collapses
	// the card — one side is zero, so there's nothing to compare.
	WorkPct                                  int
	HasSplit                                 bool
	WorkWeekStr, PrivatWeekStr, WorkMonthStr string

	// composition (Engagement/Vorhaben) — direct children, BuildComp.
	Comp []CompRow
	// chain (Repo) — this → ancestors → total, BuildChain.
	Chain []ChainRow

	// pulse + knowledge
	Pulse     []ActivityRowVM
	Docs      []UebersichtDoc
	DocsTotal int
}

// CompRow is one direct child's share of the cockpit node's subtree total.
type CompRow struct {
	ID, Name string
	Kind     domain.NodeKind
	Color    string
	DurStr   string
	Pct      int
	Live     bool   // a session is running somewhere in this child's subtree
	LastAct  string // relative time of the freshest subtree activity, "" if none
}

// ChainRow is one row in the Repo "flows upward" chain: this node (This),
// then each ancestor leaf→root, then the owner-wide Sum row.
type ChainRow struct {
	Label  string // node name; "" for the Sum row (templ resolves the i18n label)
	Kind   domain.NodeKind
	DurStr string
	Pct    int
	This   bool
	Sum    bool
}

// UebersichtDoc is one row in the "recently changed knowledge" card.
type UebersichtDoc struct {
	ID, Title, Meta string
}

// UebersichtTiles is the pure output of BuildUebersichtTiles.
type UebersichtTiles struct {
	TotalStr, WeekStr, WeekDelta, MonthStr, Earnings string
}

// BuildUebersichtTiles formats the 4 rollup tiles from a subtree rollup and
// the resolved inherited rate (nil = no rate anywhere in the chain, renders
// as "—" like rateLabel's convention). WeekDelta compares Week against
// PrevWeek using the existing FmtSaldoVerbose signed formatter ("+2h 05m" /
// "−0h 30m"); it's "" when PrevWeek is zero — a fresh node has nothing to
// compare against yet.
func BuildUebersichtTiles(roll domain.NodeRollup, rate *domain.Money) UebersichtTiles {
	t := UebersichtTiles{
		TotalStr: fmtDurHM(roll.Total),
		WeekStr:  fmtDurHM(roll.Week),
		MonthStr: fmtDurHM(roll.Month),
		Earnings: "—",
	}
	if roll.PrevWeek > 0 {
		t.WeekDelta = FmtSaldoVerbose(roll.Week - roll.PrevWeek)
	}
	if rate != nil {
		t.Earnings = rate.Mul(roll.Total).String()
	}
	return t
}

// BuildSplit derives the Work/Privat week-window split. HasSplit is false
// (collapsing the card) when either side is zero — a purely-Work or
// purely-Privat node has nothing to compare.
func BuildSplit(roll domain.NodeRollup) (workPct int, hasSplit bool, workWeekStr, privatWeekStr, workMonthStr string) {
	workWeek := roll.WorkWeek
	privatWeek := roll.Week - roll.WorkWeek
	if privatWeek < 0 {
		privatWeek = 0
	}
	hasSplit = workWeek > 0 && privatWeek > 0
	workPct = sharePct(workWeek, workWeek+privatWeek)
	return workPct, hasSplit, fmtDurHM(workWeek), fmtDurHM(privatWeek), fmtDurHM(roll.WorkMonth)
}

// BuildComp builds the composition rows for Engagement/Vorhaben cockpits:
// one row per direct child with its subtree share (Pct of nodeTotal), a
// live-dot when the running session's node falls within that child's
// subtree, and the relative time of the freshest subtree activity.
//
// runningNodeID is the node the owner's running session is currently booked
// to ("" if none). subtreeParents maps every node in the cockpit's subtree to
// its parent — this is the SAME map the caller already built from one
// Subtree() query, so live/last-activity attribution needs no extra query:
// both walk nodeID up subtreeParents until they land on one of children's
// IDs. pulse is the already subtree-filtered activity feed (FilterPulse).
func BuildComp(children []domain.Node, statsByID map[string]domain.NodeRollup, runningNodeID string, subtreeParents map[string]string, pulse []domain.ActivityEntry, nodeTotal time.Duration, now time.Time) []CompRow {
	childSet := make(map[string]bool, len(children))
	for _, c := range children {
		childSet[c.ID] = true
	}

	lastAt := make(map[string]time.Time, len(children))
	for _, e := range pulse {
		if e.NodeRef == nil {
			continue
		}
		if cid, ok := walkToChild(*e.NodeRef, childSet, subtreeParents); ok {
			if e.At.After(lastAt[cid]) {
				lastAt[cid] = e.At
			}
		}
	}

	var liveChild string
	if runningNodeID != "" {
		liveChild, _ = walkToChild(runningNodeID, childSet, subtreeParents)
	}

	rows := make([]CompRow, 0, len(children))
	for _, c := range children {
		total := statsByID[c.ID].Total
		pct := sharePct(total, nodeTotal)
		lastAct := ""
		if t, ok := lastAt[c.ID]; ok {
			lastAct = fmtRelTime(t, now)
		}
		rows = append(rows, CompRow{
			ID: c.ID, Name: c.Name, Kind: c.Kind, Color: c.Color,
			DurStr: fmtDurHM(total), Pct: pct,
			Live: c.ID == liveChild, LastAct: lastAct,
		})
	}
	return rows
}

// walkToChild walks nodeID up through parents (childID -> parentID) until it
// lands on one of childSet's IDs, returning that child's ID. ok is false if
// the walk runs off the map before finding one (nodeID isn't under any of
// these children — e.g. it IS the cockpit node itself, or lies outside the
// subtree entirely).
func walkToChild(nodeID string, childSet map[string]bool, parents map[string]string) (string, bool) {
	cur := nodeID
	for i := 0; i <= len(parents); i++ {
		if childSet[cur] {
			return cur, true
		}
		parent, ok := parents[cur]
		if !ok {
			return "", false
		}
		cur = parent
	}
	return "", false
}

// BuildChain builds the Repo "flows upward" rows: this node (This:true),
// then each ancestor leaf→root (self already excluded by the caller), then
// a Sum row carrying ownerTotal. statsByID must carry an entry for node.ID
// and every ancestor's ID (missing entries default to a zero rollup).
func BuildChain(node domain.Node, ancestors []domain.Node, statsByID map[string]domain.NodeRollup, ownerTotal time.Duration) []ChainRow {
	pct := func(d time.Duration) int { return sharePct(d, ownerTotal) }
	this := statsByID[node.ID].Total
	rows := make([]ChainRow, 0, len(ancestors)+2)
	rows = append(rows, ChainRow{Label: node.Name, Kind: node.Kind, DurStr: fmtDurHM(this), Pct: pct(this), This: true})
	for _, a := range ancestors {
		if a.ID == node.ID {
			continue // defensive: caller-provided ancestors should already exclude self
		}
		d := statsByID[a.ID].Total
		rows = append(rows, ChainRow{Label: a.Name, Kind: a.Kind, DurStr: fmtDurHM(d), Pct: pct(d)})
	}
	rows = append(rows, ChainRow{DurStr: fmtDurHM(ownerTotal), Pct: 100, Sum: true})
	return rows
}

// FilterPulse keeps only entries whose NodeRef falls within subtreeIDs —
// activity on a foreign subtree is dropped. Entries without a NodeRef (no
// node association, e.g. a plain document event) are dropped too, since
// subtree membership can't be determined for them.
func FilterPulse(entries []domain.ActivityEntry, subtreeIDs map[string]bool) []domain.ActivityEntry {
	out := make([]domain.ActivityEntry, 0, len(entries))
	for _, e := range entries {
		if e.NodeRef != nil && subtreeIDs[*e.NodeRef] {
			out = append(out, e)
		}
	}
	return out
}

// TopDocs filters docs to the subtree, sorts by UpdatedAt desc, and returns
// the top 3 plus the total count of ALL subtree-matching docs (not just the
// 3 shown) — the second return drives the "alle N ›" footer link.
func TopDocs(docs []domain.Document, subtreeIDs map[string]bool, now time.Time) ([]UebersichtDoc, int) {
	matched := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.NodeID != nil && subtreeIDs[*d.NodeID] {
			matched = append(matched, d)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].UpdatedAt.After(matched[j].UpdatedAt) })
	total := len(matched)
	if len(matched) > 3 {
		matched = matched[:3]
	}
	out := make([]UebersichtDoc, 0, len(matched))
	for _, d := range matched {
		out = append(out, UebersichtDoc{ID: d.ID, Title: d.Title, Meta: fmtRelTime(d.UpdatedAt, now)})
	}
	return out, total
}

// sharePct returns part/whole as an integer percentage, rounded half-up,
// guarding whole<=0 (→ 0). Shared by the tiles/comp/chain share bars so the
// divide-by-zero guard and rounding convention live in one place.
func sharePct(part, whole time.Duration) int {
	if whole <= 0 {
		return 0
	}
	return int(math.Round(float64(part) / float64(whole) * 100))
}

// pctStyle renders a percentage as a CSS width style for the split/chain
// bars, clamped to [0,100] (ClampPct) so a rounding overshoot never overflows.
func pctStyle(pct int) string {
	return fmt.Sprintf("width:%d%%", ClampPct(pct))
}
