package markdown_overlay

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal / sem are the package-scoped Palette views the chrome reads
// from. Initialised from theme.Default at package load; the composition
// root swaps them via SetPalette before the first New(...).
var (
	pal = theme.Default
	sem = pal.Sem()
)

// SetPalette swaps the package palette and rebuilds all styles. Call
// once at boot, before any New(...) — see cmd/flow/main.go.
func SetPalette(p theme.Palette) {
	pal = p
	sem = p.Sem()
	rebuildStyles()
}

var (
	frameStyle         lipgloss.Style
	titleStyle         lipgloss.Style
	separatorStyle     lipgloss.Style
	footerStyle        lipgloss.Style
	footerKeyStyle     lipgloss.Style
	statusBarStyle     lipgloss.Style
	statusBarPathStyle lipgloss.Style
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

	statusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(pal.BgChip)).
		Foreground(lipgloss.Color(pal.FgDim))

	statusBarPathStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(pal.BgChip)).
		Foreground(lipgloss.Color(pal.Fg))
}
