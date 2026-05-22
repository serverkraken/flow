// Package confirm provides a yes/no dialog as a bubbletea sub-model.
// Two kinds: KindDefault (yellow question, used for routine
// confirmations) and KindDanger (red question + bold, used for
// destructive operations like a delete-without-undo). The hint string
// stays the same — the kind only changes the question's colour and
// the implicit "this is serious" cue.
package confirm

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Kind selects the dialog's semantic flavour.
type Kind int

const (
	// KindDefault is the routine yes/no — yellow question.
	KindDefault Kind = iota
	// KindDanger is for destructive operations — red question, no
	// implicit confirm-on-Enter without an explicit `y`.
	KindDanger
)

// ResultMsg is sent when the user confirms or denies.
type ResultMsg struct {
	Confirmed bool
}

// KeyMap is the canonical key-binding set for a confirm dialog. Held
// on the Model and exported via Keys() so a parent's `?`-overlay can
// surface the bindings without hardcoding them. (audit A11y-5)
type KeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
}

// DefaultKeyMap returns the canonical bindings: y/Enter to confirm,
// n/Esc to cancel.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("y", "enter"),
			key.WithHelp("y/Enter", "ja"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "esc"),
			key.WithHelp("n/Esc", "nein"),
		),
	}
}

// Model is the bubbletea sub-model for a yes/no dialog.
type Model struct {
	question string
	detail   string
	kind     Kind
	keys     KeyMap
	theme    theme.Palette
}

// New creates a default-kind confirm dialog. question is shown
// prominently; detail is optional context rendered below it.
func New(question, detail string, p theme.Palette) Model {
	return Model{question: question, detail: detail, kind: KindDefault, keys: DefaultKeyMap(), theme: p}
}

// NewDanger creates a danger-kind confirm dialog. The question is
// rendered in red bold so the user reads it differently to a routine
// confirm. The hint and key handling are identical to the default.
func NewDanger(question, detail string, p theme.Palette) Model {
	return Model{question: question, detail: detail, kind: KindDanger, keys: DefaultKeyMap(), theme: p}
}

// Keys exports the active key bindings for help-overlay aggregation.
func (m Model) Keys() KeyMap { return m.keys }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles y/Enter (confirm) and n/Esc (deny).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, m.keys.Confirm):
			return m, confirmed(true)
		case key.Matches(msg, m.keys.Cancel):
			return m, confirmed(false)
		}
	}
	return m, nil
}

// View renders the dialog.
//
// Surface-Tokens (Fg/FgMuted) bleiben direkt aus der Palette — sie
// haben keine Sem-Aliase, weil sie keine Hue, sondern struktureller
// Vorder-/Hintergrund sind. Hues für Question (Yellow/Red) gehen
// dagegen über Sem.Warning/Sem.Danger.
func (m Model) View() string {
	q := lipgloss.NewStyle().
		Foreground(m.questionColor()).
		Bold(true).
		Render(m.question)
	var detail string
	if m.detail != "" {
		detail = "\n" + lipgloss.NewStyle().Foreground(m.theme.Fg).Render(m.detail)
	}
	// A11y: Default-Action explizit als `[y/Enter]` (bold + bracketed) gegen
	// die Cancel-Action (dim, ohne Brackets) absetzen. Brackets sind ein
	// non-color Signal — bei NO_COLOR weiß der User, welche Taste der
	// "primäre" Pfad ist, ohne sich auf Bold-Rendering verlassen zu müssen.
	//
	// Die confirm-spezifische Bracket-Variante ist eine A11y-Erweiterung
	// gegenüber strings.HintConfirm (footer-/statusbar-form ohne Brackets).
	// Beide Wordings sind synchronisiert; Änderungen am DE-Wording müssen
	// hier UND in components/strings.HintConfirm passieren.
	dim := lipgloss.NewStyle().Foreground(m.theme.FgMuted)
	primary := lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(true)
	hint := primary.Render("[y/Enter] → ja") + dim.Render("  ·  n/Esc → nein")
	return q + detail + "\n\n" + hint
}

// questionColor picks Sem.Warning for default, Sem.Danger for Danger
// — über den semantischen Alias statt Yellow/Red direkt zuzugreifen,
// damit ein Palette-Swap, der "Warning" auf einen anderen Hue mapt
// (z. B. eine High-Contrast-Variante), das Confirm-Modal mitnimmt.
func (m Model) questionColor() theme.Color {
	sem := m.theme.Sem()
	if m.kind == KindDanger {
		return sem.Danger
	}
	return sem.Warning
}

func confirmed(yes bool) tea.Cmd {
	return func() tea.Msg { return ResultMsg{Confirmed: yes} }
}
