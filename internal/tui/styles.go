package tui

import "charm.land/lipgloss/v2"

// Tokyonight-night palette (subset). tui-usability governs the full semantics.
var (
	colBg     = lipgloss.Color("#1a1b26")
	colMuted  = lipgloss.Color("#565f89")
	colAccent = lipgloss.Color("#7aa2f7")
	colGreen  = lipgloss.Color("#9ece6a")
	colRed    = lipgloss.Color("#f7768e")

	styleHeader  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleRunning = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Bold(true).Padding(0, 1)
	styleMuted   = lipgloss.NewStyle().Foreground(colMuted)
	styleSel     = lipgloss.NewStyle().Foreground(colBg).Background(colAccent)
	styleErr     = lipgloss.NewStyle().Foreground(colRed)
	styleOk      = lipgloss.NewStyle().Foreground(colGreen)
	styleWarn    = lipgloss.NewStyle().Foreground(colRed)
)
