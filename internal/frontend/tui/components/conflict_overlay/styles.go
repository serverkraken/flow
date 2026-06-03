package conflict_overlay

import (
	"sync/atomic"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// overlayStyles is a coherent snapshot of every lipgloss.Style this
// component uses. Held behind an atomic.Pointer so SetPalette can
// replace the whole set atomically — no half-rebuilt state visible to
// concurrent renders. Pattern mirrors markdown_overlay and project_picker.
type overlayStyles struct {
	frame       lipgloss.Style // outer rounded border
	title       lipgloss.Style // conflict title — Bold + Danger (red)
	separator   lipgloss.Style // ─── rule below title
	body        lipgloss.Style // plain body text (Fg)
	choiceKey   lipgloss.Style // "[s]", "[l]", "[t]", "[n]" — Accent-colored
	choiceLabel lipgloss.Style // choice description — Fg
	escHint     lipgloss.Style // "[esc]" hint — FgMuted
}

var stylesPtr atomic.Pointer[overlayStyles]

// styles returns the active style snapshot. Callers must treat the
// returned pointer as immutable.
func styles() *overlayStyles { return stylesPtr.Load() }

// SetPalette rebuilds all styles from p and stores the new snapshot.
// Call once at program start before constructing the first Model; safe
// to call concurrently with reads via the atomic.Pointer.
func SetPalette(p theme.Palette) {
	stylesPtr.Store(buildStyles(p))
}

// init seeds styles from theme.Default so tests that don't wire the
// composition root still render correctly.
func init() { stylesPtr.Store(buildStyles(theme.Default)) }

func buildStyles(p theme.Palette) *overlayStyles {
	sem := p.Sem()
	return &overlayStyles{
		// Rounded frame with load-bearing border (BorderStrong ≥ 3:1 WCAG).
		// Matches markdown_overlay and project_picker conventions.
		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.BorderStrong).
			Padding(0, theme.PadSM),

		// Title: Bold + Danger — the conflict title is a warning signal.
		// §Color semantics: Danger (red) for error / fail / destructive.
		title: lipgloss.NewStyle().
			Foreground(sem.Danger).
			Bold(true),

		// Separator: dim Border color — structure without competing with body.
		separator: lipgloss.NewStyle().Foreground(sem.Border),

		// Body: default Fg — load-bearing body text must never be FgMuted.
		body: lipgloss.NewStyle().Foreground(p.Fg),

		// Choice key brackets "[s]" / "[t]": Accent (blue) — interactive
		// element color. §A11y: bracket is a non-color affordance cue.
		choiceKey: lipgloss.NewStyle().Foreground(sem.Accent).Bold(true),

		// Choice label: plain Fg, not bold — key already carries the emphasis.
		choiceLabel: lipgloss.NewStyle().Foreground(p.Fg),

		// Esc hint: FgMuted — lower priority than actionable choices.
		escHint: lipgloss.NewStyle().Foreground(p.FgMuted),
	}
}
