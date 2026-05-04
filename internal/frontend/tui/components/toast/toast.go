// Package toast provides a self-dismissing transient message as a
// bubbletea sub-model. Four kinds reflect the four semantic flavours
// the audit fixes (Success / Warning / Danger / Info); each carries a
// glyph in addition to its colour so a NO_COLOR or colour-blind reader
// still gets the signal (audit A11y-2).
package toast

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// DefaultDuration is the recommended toast lifetime per the TUI usability
// skill ("~2 s default duration"). NewDefault uses it; callers that need a
// non-canonical timing pass an explicit value to New.
const DefaultDuration = 2 * time.Second

// Kind selects the toast's semantic flavour.
type Kind int

const (
	// KindSuccess — green checkmark glyph. The default kind.
	KindSuccess Kind = iota
	// KindWarning — yellow up-arrow glyph (heads-up, mild).
	KindWarning
	// KindDanger — red cross glyph (failure, attention required).
	KindDanger
	// KindInfo — cyan info glyph (neutral, no-action note).
	KindInfo
)

// DismissedMsg is sent when the toast auto-dismisses. The id discriminates
// the dismiss target: each toast Model is created with a fresh monotonic
// id, and Update only acts on a DismissedMsg whose id matches its own.
// Without this, an old toast's already-scheduled tea.Tick would
// prematurely dismiss the next toast that overwrote it on the screen.
//
// A zero-id DismissedMsg is treated as a wildcard dismiss for
// back-compat with callers that construct the message manually.
type DismissedMsg struct{ id uint64 }

// Dismiss returns the DismissedMsg that matches m. Tests and callers
// that synthesize a dismiss without going through Init's tea.Tick
// (e.g. an Esc-to-dismiss handler) build the message via this.
func (m Model) Dismiss() DismissedMsg {
	return DismissedMsg{id: m.id}
}

// nextToastID is bumped on every constructor so each instance owns its
// own dismissal id. uint64 wraparound is not a practical concern for an
// interactive TUI.
var nextToastID uint64

func newID() uint64 {
	nextToastID++
	return nextToastID
}

// Model is the bubbletea sub-model for a toast notification.
type Model struct {
	id      uint64
	text    string
	dur     time.Duration
	kind    Kind
	visible bool
	theme   theme.Palette
}

// New creates a Success-kind toast that auto-dismisses after dur.
// Kept for back-compat; prefer NewSuccess / NewWarning / NewDanger /
// NewInfo to make the semantic flavour explicit at the call-site.
func New(text string, dur time.Duration, p theme.Palette) Model {
	return Model{id: newID(), text: text, dur: dur, kind: KindSuccess, visible: true, theme: p}
}

// NewDefault creates a Success toast with the canonical DefaultDuration.
// Prefer this over New unless a specific timing is part of the screen's
// behaviour (e.g. „long action just finished, give the user a beat to
// read the result").
func NewDefault(text string, p theme.Palette) Model {
	return New(text, DefaultDuration, p)
}

// NewSuccess constructs a green ✓-toast with DefaultDuration.
func NewSuccess(text string, p theme.Palette) Model {
	return Model{id: newID(), text: text, dur: DefaultDuration, kind: KindSuccess, visible: true, theme: p}
}

// NewWarning constructs a yellow ▲-toast with DefaultDuration.
func NewWarning(text string, p theme.Palette) Model {
	return Model{id: newID(), text: text, dur: DefaultDuration, kind: KindWarning, visible: true, theme: p}
}

// NewDanger constructs a red ✗-toast with DefaultDuration.
func NewDanger(text string, p theme.Palette) Model {
	return Model{id: newID(), text: text, dur: DefaultDuration, kind: KindDanger, visible: true, theme: p}
}

// NewInfo constructs a cyan ›-toast with DefaultDuration.
func NewInfo(text string, p theme.Palette) Model {
	return Model{id: newID(), text: text, dur: DefaultDuration, kind: KindInfo, visible: true, theme: p}
}

// Visible reports whether the toast is still showing.
func (m Model) Visible() bool { return m.visible }

// Init starts the dismiss timer keyed off this toast's id, so an old
// toast's pending tick never dismisses a newer one.
func (m Model) Init() tea.Cmd {
	id := m.id
	return tea.Tick(m.dur, func(time.Time) tea.Msg { return DismissedMsg{id: id} })
}

// Update hides the toast on a matching DismissedMsg. Mismatched ids
// (a tick from a previously-shown toast that has since been replaced)
// are ignored. A zero-id DismissedMsg dismisses any toast — kept for
// callers and tests that construct DismissedMsg{} directly.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if d, ok := msg.(DismissedMsg); ok && (d.id == 0 || d.id == m.id) {
		m.visible = false
	}
	return m, nil
}

// View renders the toast or an empty string when dismissed.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	glyph, color := m.glyphAndColor()
	return lipgloss.NewStyle().Foreground(color).Bold(true).
		Render(glyph + " " + m.text)
}

// glyphAndColor maps Kind to (glyph, foreground colour). Kept as a
// single switch so a future Kind addition is one block to extend.
func (m Model) glyphAndColor() (string, lipgloss.Color) {
	switch m.kind {
	case KindWarning:
		return "▲", m.theme.Yellow
	case KindDanger:
		return "✗", m.theme.Red
	case KindInfo:
		return "›", m.theme.Cyan
	default:
		return "✓", m.theme.Green
	}
}
