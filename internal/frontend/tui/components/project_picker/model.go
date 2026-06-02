package project_picker

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// chromeRows is the number of non-list rows the picker renders:
// title border (1) + filter line (1) + separator (1) + footer (1) + bottom border (1).
// Used when computing the maximum visible list rows.
const chromeRows = 5

// Model is the bubbletea model for the project picker. It takes over the
// full screen until the user picks a project, creates one, or presses Esc.
//
// Construct via New; pass terminal dimensions via SetSize; route all
// tea.Msg through Update; render via View.
type Model struct {
	// items is the full list supplied by the caller (MRU-sorted expected).
	items []domain.Project
	// filtered is the score-sorted subset matching the current filter,
	// or equal to items when the filter is empty.
	filtered []domain.Project
	// highlights holds the fuzzy-matched rune indices parallel to filtered.
	highlights [][]int
	// filter is the current free-text filter string.
	filter string
	// cursor is the current selection index in the range
	// [0, len(filtered)]. Index len(filtered) points to the
	// sticky "+ Neues Projekt anlegen" pseudo-row.
	cursor int
	// width and height are the terminal dimensions, set via SetSize.
	width, height int

	palette theme.Palette

	// Callbacks — injected by the caller, never nil after New.
	onPick   func(domain.Project) tea.Msg
	onCreate func(name string) tea.Msg
	onCancel tea.Msg
}

// New constructs a Model. items is the full project list (caller provides
// MRU ordering); palette drives all colors; onPick/onCreate/onCancel are
// the domain-action callbacks (must not be nil).
func New(
	items []domain.Project,
	p theme.Palette,
	onPick func(domain.Project) tea.Msg,
	onCreate func(string) tea.Msg,
	onCancel tea.Msg,
) Model {
	m := Model{
		items:    items,
		palette:  p,
		onPick:   onPick,
		onCreate: onCreate,
		onCancel: onCancel,
	}
	m.applyFilter()
	return m
}

// SetSize sets the terminal dimensions for layout. Returns a new Model.
// Call on the first tea.WindowSizeMsg and whenever the terminal resizes.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// Init satisfies the bubbletea.Model interface; the picker has no async
// startup work.
func (m Model) Init() tea.Cmd { return nil }

// Cursor returns the current cursor index. Exported for testing.
func (m Model) Cursor() int { return m.cursor }

// Filter returns the current filter string. Exported for testing.
func (m Model) Filter() string { return m.filter }

// neuIdx returns the index of the sticky "+ Neues Projekt anlegen" entry
// — always len(filtered), one past the last real item.
func (m Model) neuIdx() int { return len(m.filtered) }

// applyFilter rebuilds filtered + highlights from items using the current
// filter string. When the filter is empty the full items slice is used
// unchanged (no allocation). The cursor is clamped to the new list length.
func (m *Model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.items
		m.highlights = make([][]int, len(m.items))
	} else {
		names := make([]string, len(m.items))
		for i, p := range m.items {
			names[i] = p.Name
		}
		matches := fuzzy.Find(m.filter, names)
		m.filtered = make([]domain.Project, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.filtered[i] = m.items[match.Index]
			m.highlights[i] = match.MatchedIndexes
		}
	}
	// Clamp cursor to [0, neuIdx()].
	if m.cursor > m.neuIdx() {
		m.cursor = m.neuIdx()
	}
}
