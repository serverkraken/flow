// Package strings holds the canonical user-facing strings of the TUI
// (German). docs/design-system-audit.md §2.1: a confirm dialog, a
// toast, and a status hint that all mean "press y to confirm" must
// say it the same way — centralising the literals here makes that a
// compile-time guarantee instead of a copy-paste discipline.
//
// Anything Flow's TUI shows the user that isn't generated content
// (note title, session name, …) belongs here once it appears in two
// or more places.
package strings

// Hint strings — short, single-line, comma-separated keys. Used in
// status-bar Hints components and footer rows.
const (
	HintConfirm = "y/Enter → ja  ·  n/Esc → nein"
	HintCancel  = "Esc → abbrechen"
	HintFilter  = "/ → suchen"
	HintHelp    = "? → Hilfe"
	HintQuit    = "q → schließen"
	HintNav     = "j/k → navigieren  ·  Enter → wählen"
)

// Label strings — block-level text rendered inside boxes / cards.
const (
	LabelLoading      = "lädt …"
	LabelEmpty        = "Keine Treffer."
	LabelError        = "Fehler:"
	LabelNoSelection  = "Keine Auswahl."
	LabelConfirmTitle = "Bestätigen"
)
