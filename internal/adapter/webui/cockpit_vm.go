package webui

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// CockpitTimerState is the per-node timer state shown in the cockpit head.
type CockpitTimerState int

const (
	TimerNotBookable CockpitTimerState = iota // branch (or non-bookable kind)
	TimerIdle                                 // bookable, nothing running → [Start]
	TimerHere                                 // running on THIS node → [Stop] + live clock
	TimerOtherBound                           // running on another node → [Switch]
	TimerUnbound                              // running but unbooked (no node) → link Home, no switch
)

// CockpitTimer is the resolved timer view state for the head.
type CockpitTimer struct {
	State       CockpitTimerState
	RunningID   string // running session id (hidden field for stop/switch)
	RunningBase int    // elapsed seconds at render (data-base for the live clock)
	OtherID     string // node id the timer runs on (OtherBound) — link target
	OtherName   string // node name the timer runs on (OtherBound)
}

// NodeTimer computes the timer state for nodeID given the owner's running
// session (nil = none). Pure: unit-tested in cockpit_vm_test.go.
func NodeTimer(running *domain.WorkSession, nodeID string, bookable bool, now time.Time, nameOf func(id string) string) CockpitTimer {
	if !bookable {
		return CockpitTimer{State: TimerNotBookable}
	}
	if running == nil {
		return CockpitTimer{State: TimerIdle}
	}
	base := int(running.Elapsed(now).Seconds())
	if running.NodeID == nil {
		return CockpitTimer{State: TimerUnbound, RunningID: running.ID, RunningBase: base}
	}
	if *running.NodeID == nodeID {
		return CockpitTimer{State: TimerHere, RunningID: running.ID, RunningBase: base}
	}
	return CockpitTimer{
		State: TimerOtherBound, RunningID: running.ID, RunningBase: base,
		OtherID: *running.NodeID, OtherName: nameOf(*running.NodeID),
	}
}

// NodeChild is one direct child row in the Struktur tab.
type NodeChild struct {
	N     domain.Node
	Total string // mini worktime label (e.g. "12:30 h"), "" if zero
}

// NodeCockpit drives the cockpit page, the rail fragment, and the active tab panel.
type NodeCockpit struct {
	User            string
	N               domain.Node
	Ancestors       []domain.Node // leaf→root, self included (NodeStore.Ancestors order)
	DescriptionHTML template.HTML
	Today           string // YYYY-MM-DD, today's date — Nachbuchen dialog prefill
	// rail: subtree rollup + inherited rate + identity/timer extras
	Rollup       domain.NodeRollup // Total, Week, Month durations
	Earnings     string            // ResolveRate(chain) × Total, "" if no rate in chain
	Rate         string            // inherited rate label, "" if none
	Timer        CockpitTimer
	LogoShape    string   // ""|"hex"|"tile" — LogoShape(w,h) of the uploaded logo, if any
	TodayHere    string   // today's own-node time (fmtDurHM), NOT subtree
	CountsWork   bool     // effective Work/Privat flag (domain.ResolveCountsTowardTarget)
	Contributors []string // distinct actors active in the subtree; filled by T5, empty until then
	TabCounts    map[string]int
	// active tab + its data (only the active tab's slice is populated)
	ActiveTab   string                  // uebersicht|worktime|wissen|struktur|bindings
	Uebersicht  UebersichtVM            // uebersicht: rollup tiles, split, comp/chain, pulse, docs
	SessionRows []CockpitSessionRow     // worktime: precomputed display rows, newest first
	Docs        []domain.Document       // wissen
	Children    []NodeChild             // struktur
	MoveTargets []domain.Node           // struktur reparent
	Bindings    []domain.ProjectBinding // bindings
	PanelErr    string                  // inline panel error (Nachbuchen validation, bindings)
	// EditSession is set by fillPanelData when the worktime tab's ?edit={sid}
	// query resolves to one of the owner's sessions — it drives the edit-mode
	// SessionDialog (sessionDialogEditVM), rendered pre-opened in the panel.
	// nil when not editing.
	EditSession *domain.WorkSession
}

// CockpitTabs is the fixed tab order/keys for the strip — Übersicht is the
// default landing (see NormalizeTab).
var CockpitTabs = []struct{ Key, LabelKey string }{
	{"uebersicht", "cockpit.tab.uebersicht"},
	{"worktime", "cockpit.tab.worktime"},
	{"wissen", "cockpit.tab.wissen"},
	{"struktur", "cockpit.tab.struktur"},
	{"bindings", "cockpit.tab.bindings"},
}

// sessionDialogAddVM builds the add-mode SessionDialogVM for the ONE session
// dialog mounted once per cockpit page (the Quick Actions "Nachbuchen" button
// opens it via data-dialog-open="session-dialog"). Field names match the
// existing Nachbuchen endpoint contract.
func sessionDialogAddVM(d NodeCockpit) components.SessionDialogVM {
	return components.SessionDialogVM{
		DialogID: "session-dialog",
		Mode:     "add",
		Action:   "/nodes/" + d.N.ID + "/sessions",
		Target:   "#cockpit-main",
		Date:     d.Today,
	}
}

// sessionDialogEditVM builds the edit-mode SessionDialogVM for d.EditSession
// (resolved by fillPanelData from the worktime tab's ?edit={sid} query),
// rendered pre-opened (Open: true) so the round-trip GET lands the user
// directly inside the dialog. Action always targets d.N.ID (the currently
// VIEWED cockpit — so re-rendering returns to the right panel) but NodeID
// carries the session's OWN booked node: a containment view (Engagement's
// subtree list, Spec §4) may be showing a session actually booked on a
// descendant Repo, and editing its times must not silently reassign it up to
// the viewed node. Since Nodes stays empty (no reassignment picker in this
// task), sessionDialogBody renders NodeID as a hidden field instead.
func sessionDialogEditVM(d NodeCockpit) components.SessionDialogVM {
	sess := d.EditSession
	nodeID := ""
	if sess.NodeID != nil {
		nodeID = *sess.NodeID
	}
	to := ""
	if sess.Stop != nil {
		to = sess.Stop.Format("15:04")
	}
	return components.SessionDialogVM{
		DialogID: "session-dialog-edit",
		Mode:     "edit",
		Action:   "/nodes/" + d.N.ID + "/sessions/" + sess.ID + "/edit",
		Target:   "#cockpit-main",
		Open:     true,
		Date:     sess.Start.Format("2006-01-02"),
		From:     sess.Start.Format("15:04"),
		To:       to,
		Tag:      strings.Join(sess.Tags, " "),
		Note:     sess.Note,
		NodeID:   nodeID,
	}
}


// CockpitSessionRow is a precomputed display row for the worktime panel.
// Fields are formatted strings to keep template logic-free.
type CockpitSessionRow struct {
	ID       string          // session id — edit link + delete form target
	Date     string          // e.g. "Sa 27.06."
	Span     string          // e.g. "14:00–16:00"
	Tag      string          // space-joined tags, "" if none
	Dur      string          // e.g. "2:00 h"
	Running  bool            // true if session has no Stop time
	NodeID   string          // booked node id — every row has one (unbooked sessions never list here)
	NodeName string          // booked node name — the containment pill's label
	NodeKind domain.NodeKind // booked node kind — the containment pill's glyph/tone
}

// BuildCockpitSessionRows converts a WorkSession slice (newest-first) to display
// rows. now is used to compute elapsed for running sessions. names/kinds map a
// node id (the subtree the caller resolved — Spec §4 containment) to its
// display name/kind for the row's node-pill; a session whose node isn't in
// either map (shouldn't happen — every row's node comes from the same subtree
// query) degrades to an empty pill rather than panicking.
func BuildCockpitSessionRows(sessions []domain.WorkSession, now time.Time, names map[string]string, kinds map[string]domain.NodeKind) []CockpitSessionRow {
	rows := make([]CockpitSessionRow, 0, len(sessions))
	for _, s := range sessions {
		span := s.Start.Format("15:04") + "–"
		if s.Stop != nil {
			span += s.Stop.Format("15:04")
		} else {
			span += "…"
		}
		nodeID := ""
		if s.NodeID != nil {
			nodeID = *s.NodeID
		}
		rows = append(rows, CockpitSessionRow{
			ID:       s.ID,
			Date:     s.Start.Format("Mon 02.01."),
			Span:     span,
			Tag:      strings.Join(s.Tags, " "),
			Dur:      fmtDurHM(s.Elapsed(now)),
			Running:  s.Stop == nil,
			NodeID:   nodeID,
			NodeName: names[nodeID],
			NodeKind: kinds[nodeID],
		})
	}
	return rows
}

// cockpitPanelSSE returns the hx-trigger SSE event list for a tab's live reload.
func cockpitPanelSSE(tab string) string {
	switch tab {
	case "uebersicht":
		return "sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:activity.logged, sse:document.updated, sse:node.updated"
	case "worktime":
		return "sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted"
	case "wissen":
		return "sse:document.created, sse:document.updated, sse:document.deleted"
	case "struktur":
		return "sse:node.created, sse:node.updated, sse:node.moved, sse:node.deleted"
	default:
		return "" // bindings: reload only after own mutation
	}
}

// NormalizeTab returns a valid tab key, defaulting to "uebersicht" (the
// living-project-home landing).
func NormalizeTab(tab string) string {
	for _, t := range CockpitTabs {
		if t.Key == tab {
			return tab
		}
	}
	return "uebersicht"
}

// FmtDurHMExport renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
// Exported so the httpserver adapter can format child worktime totals.
func FmtDurHMExport(d time.Duration) string { return fmtDurHM(d) }

// fmtDurHM renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
func fmtDurHM(d time.Duration) string {
	m := int(d.Minutes())
	if m < 0 {
		m = 0
	}
	return fmt.Sprintf("%d:%02d h", m/60, m%60)
}

// cockpitAccent maps a node colour name to the left accent-bar class.
func cockpitAccent(color string) string {
	switch color {
	case "cyan":
		return "bg-cyan"
	case "purple":
		return "bg-purple"
	case "green":
		return "bg-green"
	case "blue":
		return "bg-blue"
	case "orange":
		return "bg-orange"
	default:
		return "bg-blue"
	}
}

// cockpitTileClass maps a node colour to the hex tile wash+text class.
// Delegates to heuteTileClass which already maps all known hues.
func cockpitTileClass(color string) string {
	return heuteTileClass(color)
}

// cockpitRateSource returns the name of the nearest ancestor that actually
// carries a rate — matching domain.ResolveRate's leaf→root walk — or "" when
// none does. The "geerbt von" label must name the real source, not the root:
// a rate set on an intermediate Vorhaben otherwise mis-attributes to the
// Engagement in a 3+-level chain.
func cockpitRateSource(d NodeCockpit) string {
	for _, a := range d.Ancestors {
		if a.ID == d.N.ID {
			continue // the ancestor chain may include the cockpit node itself
		}
		if a.Rate != nil {
			return a.Name
		}
	}
	return ""
}

// glyphOrDefault returns the node glyph or a default identity glyph when unset.
// (Named distinctly from httpserver.glyphOr to avoid cross-package confusion.)
func glyphOrDefault(g string) string {
	if g == "" {
		return "◆"
	}
	return g
}
