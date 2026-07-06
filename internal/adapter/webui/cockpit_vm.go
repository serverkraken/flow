package webui

import (
	"context"
	"fmt"
	"html/template"
	"strconv"
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
	// ChainRootName/ChainRootTotal feed the instr-band's third stats segment
	// (the root engagement's whole-chain total, Mockup "RTL Extern 304:46 h").
	// Filled by Task 7's page wiring; "" until then — cockpitStatsLine falls
	// back to the generic "Kette" i18n label + "—" so the line still renders.
	ChainRootName  string
	ChainRootTotal string // fmtDurHM-formatted, "" = unknown/not yet wired
	// ChainStats carries the per-ancestor-level rollup the Meta-Spalte's Kette
	// block needs (BuildChain wants one NodeRollup per node.ID: N + every
	// ancestor). Filled by Task 7's page wiring via s.Stats.NodeStats; empty
	// map means every chain row renders "—" (Nullen ohne Bühne, Spec §4).
	ChainStats  map[string]domain.NodeRollup
	Pulse       []ActivityRowVM         // subtree-filtered live activity feed (Puls section)
	SessionRows []CockpitSessionRow     // Buchungen: precomputed display rows, newest first
	WissenRows  []WissenRow             // Wissen: built display rows (BuildWissenRows), what CockpitMain renders
	WissenScope string                  // "subtree"|"self" — effective Wissen scope (drives the .seg toggle)
	WissenTotal int                     // Wissen: doc count before the section cap — drives the "Alle N ›" more-link
	WissenAll   bool                    // Wissen: true when ?wissen=all expanded the section past the cap
	Children    []NodeChild             // Enthält
	Bindings    []domain.ProjectBinding // rail Bindings block
	PanelErr    string                  // inline error surfaced on #cockpit-main or #cockpit-rail
	// EditSession is set by nodeCockpitData when the ?edit={sid} query resolves
	// to one of the owner's sessions — it drives the edit-mode SessionDialog
	// (sessionDialogEditVM), rendered pre-opened. nil when not editing.
	EditSession *domain.WorkSession
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

// FmtDurHMExport renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
// Exported so the httpserver adapter can format child worktime totals.
func FmtDurHMExport(d time.Duration) string { return fmtDurHM(d) }

// cockpitMainReloadURL returns #cockpit-main's hx-get URL for its SSE live
// reload. It preserves the Wissen section's "self" scope (?scope=self) so a
// session/document/node SSE event doesn't silently revert the user's "Nur
// dieser Knoten" toggle back to the subtree default — the same contract the
// old (now-deleted) per-tab cockpitPanelReloadURL guaranteed.
func cockpitMainReloadURL(d NodeCockpit) string {
	return cockpitMainURL(d, d.WissenAll)
}

// cockpitMainURL builds a /main fragment URL carrying the section's sticky
// query state: the Wissen "self" scope and — when all=true — the expanded
// ?wissen=all view, so neither an SSE reload nor the "Alle N ›" link drops
// the other toggle.
func cockpitMainURL(d NodeCockpit, all bool) string {
	url := "/nodes/" + d.N.ID + "/main"
	var q []string
	if d.WissenScope == "self" {
		q = append(q, "scope=self")
	}
	if all {
		q = append(q, "wissen=all")
	}
	if len(q) > 0 {
		url += "?" + strings.Join(q, "&")
	}
	return url
}

// fmtDurHM renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
func fmtDurHM(d time.Duration) string {
	m := int(d.Minutes())
	if m < 0 {
		m = 0
	}
	return fmt.Sprintf("%d:%02d h", m/60, m%60)
}

// SpineCrumbs returns the cockpit head's "up" crumb chain: every ancestor
// EXCEPT self, root→leaf order (self renders as the <h1>, not a crumb link).
// Derived from the same nodeCrumbs data as the old breadcrumb, minus the
// trailing self segment.
func SpineCrumbs(d NodeCockpit) []components.Crumb {
	all := nodeCrumbs(d)
	if len(all) <= 1 {
		return nil // no ancestors (or the defensive self-only fallback)
	}
	return all[:len(all)-1]
}

// cockpitStatusWord maps a node status to its i18n KEY (not the resolved
// label) — the VM stays domain-free/i18n-free, the templ resolves it via
// components.T. Deliberately NOT StatusBadge: its amber/slate/emerald chip
// classes are non-token colors banned on Lesesaal surfaces (Spec §7).
func cockpitStatusWord(s domain.NodeStatus) string {
	switch s {
	case domain.NodePaused:
		return "node.status.paused"
	case domain.NodeArchived:
		return "node.status.archived"
	default:
		return "node.status.active"
	}
}

// dashIfZeroDur renders a fmtDurHM-formatted duration string as "—" when it's
// empty or the zero value ("0:00 h") — "Nullen ohne Bühne" (Spec §4).
func dashIfZeroDur(s string) string {
	if s == "" || s == "0:00 h" {
		return "—"
	}
	return s
}

// KetteRow is one row of the Meta-Spalte's Kette block — a pure-string view
// of ChainRow, ready for the templ to range over (see cockpit_rail.templ).
type KetteRow struct {
	Label    string
	HoursStr string
	Here     bool // true for the leaf "this node" row (gets the "(hier)" suffix)
	Href     string
}

// ChainRows adapts BuildChain (cockpit_uebersicht_vm.go) for the Meta-Spalte
// rail: this node → ancestors leaf→root → a final "rate inherited" row built
// from d.Rate instead of BuildChain's percentage Sum row. Reine Strings,
// Nullen → "—" (dashIfZeroDur).
//
// ownerTotal (BuildChain's %-basis) isn't wired yet — Task 7's page handler
// owns that source; until then it falls back to d.Rollup.Total so every row
// still renders sane numbers.
func ChainRows(ctx context.Context, d NodeCockpit) []KetteRow {
	ownerTotal := d.Rollup.Total
	chain := BuildChain(d.N, d.Ancestors, d.ChainStats, ownerTotal)

	// ids mirrors BuildChain's own row order (this, then ancestors leaf→root
	// with self defensively excluded) so each non-Sum row gets its link target.
	ids := make([]string, 0, len(chain))
	ids = append(ids, d.N.ID)
	for _, a := range d.Ancestors {
		if a.ID == d.N.ID {
			continue
		}
		ids = append(ids, a.ID)
	}

	rows := make([]KetteRow, 0, len(chain))
	for i, c := range chain {
		if c.Sum {
			rate := d.Rate
			if rate == "" {
				rate = "—"
			}
			rows = append(rows, KetteRow{Label: components.T(ctx, "cockpit.rail.rateInherited"), HoursStr: rate})
			continue
		}
		label := ShortName(c.Label)
		href := ""
		if i < len(ids) {
			href = "/nodes/" + ids[i]
		}
		if c.This {
			label += " " + components.T(ctx, "cockpit.rail.here")
		}
		rows = append(rows, KetteRow{Label: label, HoursStr: dashIfZeroDur(c.DurStr), Here: c.This, Href: href})
	}
	return rows
}

// WissenRow is one display row in the cockpit's Wissen section (cockpit_main.templ):
// a document rendered as a type-chip + title + meta line + estimated reading
// time — the built counterpart to the raw domain.Document slice (d.Docs).
type WissenRow struct {
	ID, Title, ChipClass, ChipLabel, Meta, ReadTime string
}

// BuildWissenRows maps documents to Wissen-section display rows: ChipClass/
// ChipLabel from DocTypeChipClass/DocTypeLabel (Spec §7.1), Meta = relative
// update time + path (Spec §16.9 "Akteur · Zeit · Pfad" — domain.Document
// carries no last-editor field, verified via `rg "type Document struct"
// internal/domain/`, so Meta degrades to "Zeit · Pfad"), ReadTime = word
// count/220 minutes, rounded up to at least 1 — "" when Body is empty (a
// list query without full bodies degrades gracefully, no blocker).
func BuildWissenRows(docs []domain.Document, now time.Time) []WissenRow {
	rows := make([]WissenRow, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, WissenRow{
			ID:        doc.ID,
			Title:     doc.Title,
			ChipClass: DocTypeChipClass(doc.Type),
			ChipLabel: DocTypeLabel(doc.Type),
			Meta:      FmtRelTime(doc.UpdatedAt, now) + " · " + doc.Path,
			ReadTime:  readTimeLabel(doc.Body),
		})
	}
	return rows
}

// readTimeLabel estimates reading time at 220 words/minute (Spec §16.9),
// rounding up to at least 1 minute. Empty body yields "" (no readtime box).
func readTimeLabel(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	mins := len(strings.Fields(body)) / 220
	if mins < 1 {
		mins = 1
	}
	return strconv.Itoa(mins) + " min"
}
