// Package view styles. Mirrors browse's chrome (rounded blue frame,
// muted separators) so the viewer reads as the same application from
// the same palette — colors come from internal/frontend/tui/theme.
package view

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
)

var (
	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(theme.Blue)).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.BgHighlight))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Cyan)).
			Bold(true)

	searchActiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Yellow)).
				Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(theme.BgHighlight)).
			Foreground(lipgloss.Color(theme.FgDim))

	statusBarModeSearchStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(theme.Yellow)).
					Foreground(lipgloss.Color(theme.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarPathStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(theme.BgHighlight)).
				Foreground(lipgloss.Color(theme.Fg))

	// matchBarStyle renders the left-margin bar prepended to lines that
	// matched the search query. Inline highlight of just the matched
	// word would have to splice SGR codes into glamour output without
	// breaking nested OSC 8 hyperlinks — fragile. A two-cell left bar
	// stays robust regardless of what the line carries.
	matchBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Yellow))

	// matchCurrentBarStyle marks the cursor's current match. Same shape
	// as matchBar but bold + Orange so n/N navigation is readable.
	matchCurrentBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Orange)).
				Bold(true)

	emptyLineMarker = "  " // two cells; reserved gutter when no match
)
