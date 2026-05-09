// Package view styles. Mirrors browse's chrome (rounded blue frame,
// muted separators) so the viewer reads as the same application from
// the same palette — colors come from internal/frontend/tui/theme.
package view

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal ist die canonical Palette dieses Packages. Init beim Package-
// Load aus theme.Default; ein Runtime-Swap (Stufe-7-Goal) wired pal
// als per-render-Param und beseitigt diesen Package-State.
//
// sem ist die Sem()-Sicht auf pal — Components konsumieren laut
// docs/design-system.md den semantischen Alias (`sem.Accent`,
// `sem.Danger` …), nicht die rohe Hue. Ein zukünftiger Palette-Swap,
// der z. B. "Warning" auf Orange remappt, schiebt damit hier durch.
var (
	pal = theme.Default
	sem = pal.Sem()
)

var (
	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.Accent).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.BgChip))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(sem.Active).
			Bold(true)

	searchActiveLabelStyle = lipgloss.NewStyle().
				Foreground(sem.Warning).
				Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(pal.BgChip)).
			Foreground(lipgloss.Color(pal.FgDim))

	statusBarModeSearchStyle = lipgloss.NewStyle().
					Background(sem.Warning).
					Foreground(lipgloss.Color(pal.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarPathStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(pal.BgChip)).
				Foreground(lipgloss.Color(pal.Fg))

	// matchBarStyle renders the left-margin bar prepended to lines that
	// matched the search query. Inline highlight of just the matched
	// word would have to splice SGR codes into glamour output without
	// breaking nested OSC 8 hyperlinks — fragile. A two-cell left bar
	// stays robust regardless of what the line carries.
	matchBarStyle = lipgloss.NewStyle().
			Foreground(sem.Warning)

	// matchCurrentBarStyle marks the cursor's current match. Sem hat
	// keinen Orange-Alias (Orange ist eine Markdown-Domain-Hue, kein
	// semantisches Token); pal.Orange bleibt direkt.
	matchCurrentBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Orange)).
				Bold(true)

	emptyLineMarker = "  " // two cells; reserved gutter when no match
)
