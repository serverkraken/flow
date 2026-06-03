package conflict_overlay

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Chrome budget constants — kept in sync with the visual layout.
//
// chromeVertical = 2 (rounded border top+bottom) + 1 (title row) +
//
//	1 (separator below title) + 1 (blank line after body) +
//	1 (blank line before hints) = at minimum 6 rows beyond the
//	body + hint lines. We use a simpler fixed constant: border(2)
//	+ title(1) + sep(1) + pad(2) + hints(3) = 9.
const (
	chromeVertical    = 9  // minimum rows required to render anything useful
	contentLineBudget = 4  // 2 border + 2 padding (left + right)
	minWidth          = 30 // below this the layout degrades too much to be useful
)

// Variant selects the layout and key bindings.
type Variant int

const (
	// VariantSessionEdit is the overlay for a sessions push 409:
	// [s] Server-Version übernehmen / [l] Lokal überschreiben / [esc] Abbrechen.
	VariantSessionEdit Variant = iota
	// VariantActiveRace is the overlay for an active_sessions 409:
	// [t] Übernehmen / [n] Neue Session / [esc] Abbrechen.
	VariantActiveRace
)

// CancelMsg is emitted when the user presses Esc without choosing a
// resolution. The host model must observe this in its own Update and
// clear its overlay-state field.
type CancelMsg struct{}

// choice is a single resolvable option in the overlay.
type choice struct {
	key      string // single-character keyboard trigger, e.g. "s", "l", "t", "n"
	label    string // display label, e.g. "Server-Version übernehmen"
	callback func() tea.Msg
}

// Model is the bubbletea model for the conflict overlay. Construct via
// NewSessionEditConflict or NewActiveRaceConflict; set dimensions via
// SetSize on WindowSizeMsg; route messages via Update; render via View.
type Model struct {
	variant Variant
	title   string
	body    string // pre-formatted plain-text body (not markdown)
	choices []choice
	palette theme.Palette
	width   int
	height  int
}

// NewSessionEditConflict builds the overlay for a sessions push 409.
// onResolve is called with accept=true for "Server-Version übernehmen"
// and accept=false for "Lokal überschreiben". It must not be nil.
func NewSessionEditConflict(
	local, server domain.Session,
	p theme.Palette,
	onResolve func(accept bool) tea.Msg,
) Model {
	body := formatSessionEditBody(local, server)
	return Model{
		variant: VariantSessionEdit,
		title:   "Sync-Konflikt",
		body:    body,
		palette: p,
		choices: []choice{
			{
				key:      "s",
				label:    "Server-Version übernehmen",
				callback: func() tea.Msg { return onResolve(true) },
			},
			{
				key:      "l",
				label:    "Lokal überschreiben",
				callback: func() tea.Msg { return onResolve(false) },
			},
		},
	}
}

// NewActiveRaceConflict builds the overlay for an active_sessions 409.
// onTakeover is invoked when the user picks [t]; onParallel when [n].
// Neither may be nil.
func NewActiveRaceConflict(
	server domain.ActiveSession,
	p theme.Palette,
	onTakeover func() tea.Msg,
	onParallel func() tea.Msg,
) Model {
	body := formatActiveRaceBody(server)
	return Model{
		variant: VariantActiveRace,
		title:   "Aktive Session — Konflikt",
		body:    body,
		palette: p,
		choices: []choice{
			{
				key:      "t",
				label:    "Übernehmen (Session hierher migrieren)",
				callback: onTakeover,
			},
			{
				key:      "n",
				label:    "Neue parallele Session starten",
				callback: onParallel,
			},
		},
	}
}

// SetSize sets the terminal dimensions for layout. Returns a new Model.
// Call on the first tea.WindowSizeMsg and whenever the terminal resizes.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// Init satisfies the bubbletea.Model interface; the overlay has no async
// startup work.
func (m Model) Init() tea.Cmd { return nil }
