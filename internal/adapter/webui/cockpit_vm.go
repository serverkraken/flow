package webui

import (
	"fmt"
	"html/template"
	"strings"
	"time"

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

// NodeCockpit drives the cockpit page, head fragment, and the active tab panel.
type NodeCockpit struct {
	User            string
	N               domain.Node
	Ancestors       []domain.Node // leaf→root (NodeStore.Ancestors order)
	DescriptionHTML template.HTML
	// head: subtree rollup + inherited rate
	Rollup   domain.NodeRollup // Total, Week, Month durations
	Earnings string            // ResolveRate(chain) × Total, "" if no rate in chain
	Rate     string            // inherited rate label, "" if none
	Timer    CockpitTimer
	// active tab + its data (only the active tab's slice is populated)
	ActiveTab   string                  // worktime|wissen|struktur|bindings
	Sessions    []domain.WorkSession    // worktime: own sessions, newest first
	Docs        []domain.Document       // wissen
	Children    []NodeChild             // struktur
	MoveTargets []domain.Node           // struktur reparent
	Bindings    []domain.ProjectBinding // bindings
	PanelErr    string                  // inline panel error (Nachbuchen validation, bindings)
	SessionRows []CockpitSessionRow      // worktime: precomputed display rows, newest first
}

// CockpitTabs is the fixed tab order/keys for the strip.
var CockpitTabs = []struct{ Key, LabelKey string }{
	{"worktime", "cockpit.tab.worktime"},
	{"wissen", "cockpit.tab.wissen"},
	{"struktur", "cockpit.tab.struktur"},
	{"bindings", "cockpit.tab.bindings"},
}


// CockpitSessionRow is a precomputed display row for the worktime panel.
// Fields are formatted strings to keep template logic-free.
type CockpitSessionRow struct {
	Date    string // e.g. "Sa 27.06."
	Span    string // e.g. "14:00–16:00"
	Tag     string // space-joined tags, "" if none
	Dur     string // e.g. "2:00 h"
	Running bool   // true if session has no Stop time
}

// BuildCockpitSessionRows converts a WorkSession slice (newest-first) to display rows.
// now is used to compute elapsed for running sessions.
func BuildCockpitSessionRows(sessions []domain.WorkSession, now time.Time) []CockpitSessionRow {
	rows := make([]CockpitSessionRow, 0, len(sessions))
	for _, s := range sessions {
		span := s.Start.Format("15:04") + "–"
		if s.Stop != nil {
			span += s.Stop.Format("15:04")
		} else {
			span += "…"
		}
		rows = append(rows, CockpitSessionRow{
			Date:    s.Start.Format("Mon 02.01."),
			Span:    span,
			Tag:     strings.Join(s.Tags, " "),
			Dur:     fmtDurHM(s.Elapsed(now)),
			Running: s.Stop == nil,
		})
	}
	return rows
}

// cockpitPanelSSE returns the hx-trigger SSE event list for a tab's live reload.
func cockpitPanelSSE(tab string) string {
	switch tab {
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

// NormalizeTab returns a valid tab key, defaulting to "worktime".
func NormalizeTab(tab string) string {
	for _, t := range CockpitTabs {
		if t.Key == tab {
			return tab
		}
	}
	return "worktime"
}

// fmtDurHM renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
func fmtDurHM(d time.Duration) string {
	m := int(d.Minutes())
	if m < 0 {
		m = 0
	}
	return fmt.Sprintf("%d:%02d h", m/60, m%60)
}

// fmtSecsClock renders integer seconds as the initial clock text (overwritten
// by the live-timer JS on bind). Format mirrors the [data-timer] hero output.
func fmtSecsClock(sec int) string {
	return fmt.Sprintf("%dh %02dm %02ds", sec/3600, (sec%3600)/60, sec%60)
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

// glyphOrDefault returns the node glyph or a default identity glyph when unset.
// (Named distinctly from httpserver.glyphOr to avoid cross-package confusion.)
func glyphOrDefault(g string) string {
	if g == "" {
		return "◆"
	}
	return g
}
