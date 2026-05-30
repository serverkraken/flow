// Package strings holds the canonical user-facing strings of the TUI
// (German) plus shared rendering primitives that operate on user-visible
// text (Truncate). docs/design-system-audit.md §2.1: a confirm dialog,
// a toast, and a status hint that all mean "press y to confirm" must
// say it the same way — centralising the literals here makes that a
// compile-time guarantee instead of a copy-paste discipline.
//
// Anything Flow's TUI shows the user that isn't generated content
// (note title, session name, …) belongs here once it appears in two
// or more places.
package strings

import "charm.land/lipgloss/v2"

// Hint strings — short, single-line, comma-separated keys. Used in
// status-bar Hints components and footer rows.
const (
	HintCancel = "Esc → abbrechen"
	HintFilter = "/ → suchen"
	HintHelp   = "? → Hilfe"
	HintQuit   = "q → schließen"
	HintNav    = "j/k → navigieren  ·  Enter → wählen"
	// HintConfirm: brackets um die default-Action `[y/Enter]` — A11y-6
	// non-color cue. Mirrors confirm.Model.View() one-to-one so a y/n
	// dialog and a hint-strip never disagree.
	HintConfirm = "[y/Enter] → ja  ·  n/Esc → nein"
	// HintFormNav steht für mehrfeldige Eingabedialoge (today_dialog
	// edit-form, dayoffs add-form, history_drill edit-form). Vorher
	// dreifach wortgleich inline kopiert.
	HintFormNav = "Tab/↑↓ → Feld  ·  Enter → weiter / speichern  ·  Esc → abbrechen"
	// HintInputSave steht für ein-Feld-Inputs mit optionalem Löschen
	// (Tag-, Notiz-, Korrektur-Dialog).
	HintInputSave = "Enter → speichern  ·  leer → löschen  ·  Esc → abbrechen"
	// HintScroll ist der Scroll-Tasten-Fragment für viewport-basierte
	// Sub-Views (Heute-NoteView, Brief-Overlay, Cheatsheet). Wird
	// typischerweise mit einem Close-Fragment kombiniert.
	HintScroll = "↑/↓ · PgUp/PgDn → scrollen"
	// HintSearchInput ist der canonical Footer für Live-Filter-Inputs
	// (Kompendium Such-Modus, künftige Such-Surfaces). "tippen → filtern"
	// statt "/" weil die Surface den Filter bereits aktiv hat — der User
	// braucht das Trigger-Key nicht mehr, sondern die Apply/Abort-Verben.
	HintSearchInput = "tippen → filtern  ·  Enter → anwenden  ·  Esc → abbrechen"
)

// Label strings — block-level text rendered inside boxes / cards.
const (
	LabelLoading      = "lädt …"
	LabelEmpty        = "Keine Treffer."
	LabelError        = "Fehler:"
	LabelNoSelection  = "Keine Auswahl."
	LabelConfirmTitle = "Bestätigen"
)

// Action labels — kurze German-Klartext-Beschreibungen einer Aktion,
// genutzt in Help-Tabellen ({key, action}-Pairs).
const (
	// ActionBackToPalette beschreibt die `b`-Taste, wenn sie global
	// einen Tab-Wechsel zurück zur Palette auslöst.
	ActionBackToPalette = "Zurück zur Palette"
)

// Truncate clips s to at most maxWidth visible cells, appending "…" when
// the original is wider. maxWidth ≤ 0 returns ""; maxWidth == 1 returns
// just the ellipsis.
//
// Width is measured via lipgloss.Width so ANSI escape sequences (e.g.
// pre-styled fragments) and wide runes (CJK, some box-drawing) are
// counted as their visible cells, not their byte length. This is the
// canonical truncate used by titlebox / picker / any bordered render
// path — Bubbletea Golden Rule #2 "never auto-wrap in bordered panels".
func Truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	out := make([]rune, 0, len(s))
	used := 0
	limit := maxWidth - 1 // reserve one cell for the ellipsis
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > limit {
			break
		}
		out = append(out, r)
		used += rw
	}
	return string(out) + "…"
}
