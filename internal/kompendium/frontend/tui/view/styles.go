// Package view styles. Mirrors browse's chrome (rounded blue frame,
// muted separators) so the viewer reads as the same application from
// the same palette — colors come from internal/frontend/tui/theme.
package view

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal ist die canonical Palette dieses Packages. Init beim Package-
// Load aus theme.Default; ein Composition-Root-Aufruf von SetPalette()
// swappt sie auf den Live-Wert (= tk.Load() in cli/sidekick.go +
// cmd/flow/main.go), rebuildStyles() weist alle Style-vars neu zu.
//
// sem ist die Sem()-Sicht — Components konsumieren laut
// docs/design-system.md den semantischen Alias, nicht die rohe Hue.
var (
	pal = theme.Default
	sem = pal.Sem()
)

// SetPalette swappt die Package-Palette und rebuildet alle Styles.
// Vor view.New(...)-Aufruf ausführen, sobald der Live-Pal-Wert
// verfügbar ist.
func SetPalette(p theme.Palette) {
	pal = p
	sem = p.Sem()
	rebuildStyles()
}

var (
	frameStyle               lipgloss.Style
	titleStyle               lipgloss.Style
	separatorStyle           lipgloss.Style
	footerStyle              lipgloss.Style
	footerKeyStyle           lipgloss.Style
	searchActiveLabelStyle   lipgloss.Style
	cursorStyle              lipgloss.Style
	statusBarStyle           lipgloss.Style
	statusBarModeSearchStyle lipgloss.Style
	statusBarPathStyle       lipgloss.Style

	// matchBarStyle renders the left-margin bar prepended to lines that
	// matched the search query. Inline highlight of just the matched
	// word would have to splice SGR codes into glamour output without
	// breaking nested OSC 8 hyperlinks — fragile. A two-cell left bar
	// stays robust regardless of what the line carries.
	matchBarStyle lipgloss.Style

	// matchCurrentBarStyle marks the cursor's current match. Sem hat
	// keinen Orange-Alias (Orange ist eine Markdown-Domain-Hue, kein
	// semantisches Token); pal.Orange bleibt direkt.
	matchCurrentBarStyle lipgloss.Style

	emptyLineMarker = "  " // two cells; reserved gutter when no match
)

func init() { rebuildStyles() }

func rebuildStyles() {
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
		Foreground(lipgloss.Color(pal.FgMuted))

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

	matchBarStyle = lipgloss.NewStyle().
		Foreground(sem.Warning)

	matchCurrentBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.Orange)).
		Bold(true)
}
