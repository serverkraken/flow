package webui

import (
	"fmt"
	"time"
)

// HomeVM is the view model for the Schreibtisch (Home) landing page — L4
// Task 2 replaces the Kristall dashboard (saldo tiles/burndown/activity
// feed) with four ruhige Zeilen-Sektionen: Jetzt (the one running
// timer, display-only), Weiterarbeiten (MRU bookable nodes), Zuletzt im
// Wissen (shared WissenRowVM), and Puls (shared pulseRow/pulseSection).
type HomeVM struct {
	// Today is the pagehead eyebrow's localized full date ("Donnerstag, 3.
	// Juli 2026", Mockup Z.352) — see FmtDeskDate.
	Today string

	// Now is the currently-running session's display state, nil when idle.
	Now *RunningNowVM

	// TodayLogged is the owner's total logged time today ("heute 5:12 h"),
	// shown on both the running and the idle Jetzt-row.
	TodayLogged string

	// Continue holds the MRU bookable nodes for "Weiterarbeiten" (cap 5).
	Continue []RecentNode

	// RecentWissen holds the newest documents for "Zuletzt im Wissen" (cap 5)
	// — the same WissenRowVM shared with the /wissen "Zuletzt aktualisiert"
	// row (wissen.templ wissenRow).
	RecentWissen []WissenRowVM

	// Puls holds the account-wide activity feed (cap 8), rendered with the
	// shared pulseRow/pulseSection (activity_row.templ).
	Puls []ActivityRowVM

	Err string // inline error message (surfaced when a mutation failed)

	// Start (Screen 24): Begrüßung und Kalenderwoche im Kopf, das Tagesziel
	// neben der Uhr, und drei weitere Blöcke — was Aufmerksamkeit braucht,
	// was gestern war, was angepinnt ist. Die Zahlen (Bestand, Erträge)
	// stehen am Ende: Wissen vor Zahlen.
	Greeting   string
	Week       string // "KW 34"
	TargetLine string // "Ziel 8:00 h · noch 1:48" / "Ziel erreicht" / ""
	Attention  []AttentionRow
	Yesterday  *YesterdayNote
	Pinned     []WissenRowVM
	Bestand    BestandVM
	Ertraege   *ErtraegeVM
}

// RunningNowVM is the Jetzt-panelzeile's display state of the ONE running
// timer (Spec §10 — no second Start/Stop surface). NodeID is "" for an
// unbound running session (Stop lives only on the Topbar-Pill's node-picker
// sheet then, never here — webui_timer.go handleTimerStop needs a node for
// an unbound session and would otherwise error timer.needNode).
type RunningNowVM struct {
	NodeID      string
	NodeName    string // ShortName
	NodeHref    string
	Initials    string
	Tone        string
	BaseSeconds int64
	SinceEpoch  int64 // unix epoch of the effective start (now-elapsed) — absolute data-since anchor
	SinceStr    string // "HH:MM" the session started at
	CountsWork  bool   // domain.ResolveCountsTowardTarget(chain) — bound sessions only
}

// homeCountsLabelKey resolves the Now row's Work/Privat i18n key.
func homeCountsLabelKey(countsWork bool) string {
	if countsWork {
		return "home.countsWork"
	}
	return "home.countsPrivat"
}

// homeWeekdaysDE and homeMonthsDE back FmtDeskDate — hardcoded German like
// FmtRelTime (activity_row.go) and historieMonthYear
// (webui_historie.go): the app's day/date formatting is already
// intentionally locale-fixed in those callers rather than routed through
// the i18n catalog, and this eyebrow follows the same established
// precedent instead of introducing 19 new one-off weekday/month keys for a
// single caller.
var homeWeekdaysDE = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
var homeMonthsDE = [...]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

// FmtDeskDate renders the Schreibtisch pagehead's eyebrow date, e.g.
// "Donnerstag, 3. Juli 2026" (Mockup Z.352).
func FmtDeskDate(t time.Time) string {
	return fmt.Sprintf("%s, %d. %s %d", homeWeekdaysDE[t.Weekday()], t.Day(), homeMonthsDE[t.Month()-1], t.Year())
}
