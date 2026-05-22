// Package modal renders a centred floating panel — generalised from
// the hand-rolled DoubleBorder modal in kompendium/browse/styles.go.
//
// Three kinds reflect the three semantic flavours kompendium needed:
//
//   - KindDefault — Accent-colour border. Confirmations / pickers.
//   - KindDanger  — Danger-colour border. Destructive confirms.
//   - KindSafe    — Success-colour border. "Done" / informational.
//
// All variants share BgPanel fill, DoubleBorder, and the canonical
// PadMD/PadSM (vertical/horizontal) so a modal looks like a modal in
// every screen.
//
// docs/design-system-audit.md §2.3.14.
package modal

import (
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Kind selects the modal's semantic colour.
type Kind int

const (
	// KindDefault uses the Accent colour — primary, non-destructive.
	KindDefault Kind = iota
	// KindDanger uses the Danger colour — destructive confirmations.
	KindDanger
	// KindSafe uses the Success colour — completion / informational.
	KindSafe
)

// Opts is the input for Render. Title is optional (empty omits the
// title row). Width 0 means the modal is sized to its content.
type Opts struct {
	Title string
	Kind  Kind
	Width int
}

// Render returns the styled modal box around content. content is
// passed verbatim — callers wanting a scrollable or interactive modal
// pass a pre-rendered tea-model View() string.
func Render(content string, opts Opts, p theme.Palette) string {
	bc := borderColor(opts.Kind, p)
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(bc).
		Background(p.BgPanel).
		Foreground(p.Fg).
		Padding(theme.PadMD, theme.PadSM)
	if opts.Width > 0 {
		box = box.Width(opts.Width)
	}

	body := content
	if opts.Title != "" {
		title := lipgloss.NewStyle().
			Foreground(bc).
			Bold(true).
			Render(opts.Title)
		body = title + "\n\n" + content
	}
	return box.Render(body)
}

// borderColor picks the semantic border color for a modal kind.
func borderColor(k Kind, p theme.Palette) lipgloss.Color {
	sem := p.Sem()
	switch k {
	case KindDanger:
		return sem.Danger
	case KindSafe:
		return sem.Success
	default:
		return sem.Accent
	}
}
