// Package palette implements the palette screen: a fuzzy-filterable,
// section-grouped list of all actions aggregated from enabled tmux
// plugins' menu.entries files.
//
// The screen is port-driven: PaletteReader loads + ranks entries,
// PaletteWriter mutates the persisted stats (Mark, TogglePin), and
// ports.Tmux dispatches the selected action via tmux run-shell. The
// pure ranking algorithm lives in domain.SortPaletteEntries.
//
// File-Layout (Skill §No-Monoliths):
//   - model.go  : Types, Konstruktion, State-Accessoren, applyFilter.
//   - update.go : Update-Dispatch + Key-/Pin-/Section-/Dispatch-Pfade.
//   - render.go : View und alle View-Helpers.
package palette

import (
	"regexp"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type loadedMsg struct {
	snapshot *usecase.PaletteSnapshot
	err      error
}

// dispatchedMsg fires after an external action (popup, fire-and-forget tmux
// command) was handed off to tmux. The palette stays open and shows a toast
// with the entry's label so the user gets confirmation. Pre-F-WAVE-1 this
// returned tea.Quit; that killed flow's process and made the surrounding
// sidekick pane flicker on every action.
//
// err is non-nil when RunTmuxAction or the persist call failed. The view
// surfaces it as a danger toast so the user knows the action did NOT
// take effect — silent failures previously left the user thinking the
// pin / mark / dispatch had succeeded.
type dispatchedMsg struct {
	label string
	err   error
}

// persistDoneMsg fires after a fire-and-forget persist (Mark or
// TogglePin). On error it surfaces a warning toast and keeps the UI in
// sync with reality by reloading; on success the reload picks up the
// new pin/usage state. Both Mark and TogglePin used to run inside
// Update (synchronous disk I/O on the bubbletea event loop) — a hung
// disk would freeze the whole UI.
type persistDoneMsg struct {
	err error
}

// Mode discriminates the palette's hosting context. Default is
// ModeEmbedded — the sidekick root catches SwitchScreenMsg and swaps
// the active tab inline. ModeStandalone (gesetzt durch WithStandalone)
// passt das Verhalten an einen tmux-display-popup-Aufruf an: goto.sh-
// Actions werden wie normale tmux-Kommandos durchgereicht (mit
// run-shell), und nach erfolgreichem Dispatch quittet die Palette,
// damit das Popup zugeht (CLAUDE-tmux-migration-plan §3).
type Mode int

const (
	// ModeEmbedded ist das Standardverhalten — Palette läuft im
	// Sidekick und der Root verarbeitet SwitchScreenMsg.
	ModeEmbedded Mode = iota
	// ModeStandalone ist für tmux-Popup-Aufruf via `flow palette`.
	// goto.sh-Aktionen laufen extern; Dispatch beendet das Programm.
	ModeStandalone
)

// SwitchScreenMsg is emitted when a palette entry's action is recognized as
// a flow-internal screen switch (the goto.sh deep-link pattern). The
// sidekick root catches it and updates m.current — no subshell, no flow
// restart, no flicker. Action strings that do NOT match this pattern fall
// through to the external dispatch path.
//
// Filter is optional and, when non-empty, is applied to the target screen
// via stateRestorer.WithState(Filter, 0) right after the switch. Cross-
// screen producers (e.g. projects → worktime) use this to seed a deep-link
// like "tab=history|project:<id>" so the user lands on the right sub-tab
// with the right filter already active.
type SwitchScreenMsg struct {
	Screen string
	Filter string
}

// gotoScreenRe matches the action string written by ~/.tmux/plugins/flow/goto.sh.
// Examples it must catch:
//
//	run-shell '~/.tmux/plugins/flow/goto.sh worktime'
//	run-shell "~/.tmux/plugins/flow/goto.sh projects"
//	run-shell ~/.tmux/plugins/flow/goto.sh palette
//
// The captured group is the screen name (palette / projects / worktime /
// cheatsheet / notes), validated against domain.IsValidScreen at use site.
var gotoScreenRe = regexp.MustCompile(`flow/goto\.sh\s+(\w+)`)

// Model is the bubbletea model for the palette screen.
//
// styles is the palette-bound style cache. Built once at New().
// Round4: the render hot-path (renderRow) used to allocate 4
// lipgloss.Style per visible row per frame; on a typical 20-row
// palette with 60 fps redraw that came to 240 Style-allocations
// per keystroke. Cache makes them per-Model, not per-frame.
type Model struct {
	all        []domain.PaletteEntry
	visible    []domain.PaletteEntry
	highlights [][]int // label-rune-indices to highlight per visible entry
	cursor     int
	offset     int
	filter     textinput.Model
	pal        theme.Palette
	styles     paletteStyles
	width      int
	height     int
	err        error
	loading    bool
	session    string
	stats      domain.PaletteStats

	// toast renders a transient ack after a non-screen-switch dispatch.
	// nil when no toast is active.
	toast *toast.Model

	reader *usecase.PaletteReader
	writer *usecase.PaletteWriter
	tmux   ports.Tmux

	mode Mode
}

// paletteStyles caches the lipgloss styles that depend only on the
// palette — i.e. don't vary per row or per frame. Build once at New(),
// reuse on every render. P7 (RowWithMatch) absorbed the row-specific
// label/match/labelSel/matchSel pairs; what stays is the preview row
// (hint + bar) and the separator border above the entry list.
type paletteStyles struct {
	hint   lipgloss.Style // FgMuted — preview text + renderEmptyState dim
	bar    lipgloss.Style // Sem.Accent — preview ▎ glyph
	border lipgloss.Style // Sem.Border — separator line
}

func newPaletteStyles(p theme.Palette) paletteStyles {
	sem := p.Sem()
	return paletteStyles{
		hint:   lipgloss.NewStyle().Foreground(p.FgMuted),
		bar:    lipgloss.NewStyle().Foreground(sem.Accent),
		border: lipgloss.NewStyle().Foreground(sem.Border),
	}
}

// Option mutates a Model after New(). Mit dem Pattern bleibt die
// Standard-Konstruktor-Signatur stabil und neue Hosting-Modes können
// als opt-in über die Option-Liste durchgereicht werden.
type Option func(*Model)

// WithStandalone schaltet den ModeStandalone — siehe Mode-Doku.
// Für `flow palette` (tmux-display-popup), nicht für den Sidekick.
func WithStandalone() Option {
	return func(m *Model) { m.mode = ModeStandalone }
}

// New constructs a palette Model wired against the given use cases and
// tmux dispatcher.
func New(p theme.Palette, reader *usecase.PaletteReader, writer *usecase.PaletteWriter, tmux ports.Tmux, opts ...Option) Model {
	ti := form.NewTextInput("filter…", p)
	m := Model{
		pal:     p,
		styles:  newPaletteStyles(p),
		filter:  ti,
		loading: true,
		reader:  reader,
		writer:  writer,
		tmux:    tmux,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// HelpSections returns the canonical key bindings of the palette
// screen for aggregation by the sidekick `?`-overlay. Single source
// of truth — the overlay used to maintain a parallel hand-pasted copy.
func (Model) HelpSections() []help.Section {
	return []help.Section{{
		Title: "Palette",
		Keys: [][2]string{
			{"a–z (außer j/k/g/G)", "tippen → Filter direkt"},
			{"/", "Filter explizit öffnen"},
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"] / [", "Nächste / vorige Section"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"1–9", "Direktwahl (n-ter Treffer)"},
			{".", "Pin / Unpin (→ Favoriten)"},
			{"Enter", "Ausführen"},
			{"Esc", "Filter leeren (leer: schließen im Popup)"},
		},
	}}
}

// FilterActive reports whether the text input has focus.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter value for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor position for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state. Returns
// tea.Model (not the concrete type) so the sidekick root can call this
// through its stateRestorer interface.
func (m Model) WithState(filter string, cursor int) tea.Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init kicks off the async palette load.
func (m Model) Init() tea.Cmd { return m.loadCmd() }

func (m *Model) applyFilter() {
	q := m.filter.Value()
	if q == "" {
		m.visible = m.all
		m.highlights = make([][]int, len(m.visible))
	} else {
		targets := make([]string, len(m.all))
		for i, e := range m.all {
			targets[i] = e.Section + " " + e.Label
		}
		matches := fuzzy.Find(q, targets)
		m.visible = make([]domain.PaletteEntry, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
			labelHits := fuzzy.Find(q, []string{m.visible[i].Label})
			if len(labelHits) > 0 {
				m.highlights[i] = labelHits[0].MatchedIndexes
			}
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.offset = 0
	m.ensureCursorVisible()
}

func (m Model) maxVisible() int {
	return max(1, m.height-theme.PickerChromeRows)
}

func (m *Model) ensureCursorVisible() {
	vis := m.maxVisible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}
