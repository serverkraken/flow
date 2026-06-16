package tui

import "charm.land/lipgloss/v2"

// Tokyonight-night palette (subset). tui-usability governs the full semantics.
var (
	colBg     = lipgloss.Color("#1a1b26")
	colMuted  = lipgloss.Color("#565f89")
	colAccent = lipgloss.Color("#7aa2f7")
	colGreen  = lipgloss.Color("#9ece6a")
	colRed    = lipgloss.Color("#f7768e")
	colCyan   = lipgloss.Color("#7dcfff")
	colPurple = lipgloss.Color("#bb9af7")

	styleHeader     = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleRunning    = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Bold(true).Padding(0, 1)
	styleMuted      = lipgloss.NewStyle().Foreground(colMuted)
	styleSel        = lipgloss.NewStyle().Foreground(colBg).Background(colAccent)
	styleErr        = lipgloss.NewStyle().Foreground(colRed)
	styleOk         = lipgloss.NewStyle().Foreground(colGreen)
	styleWarn       = lipgloss.NewStyle().Foreground(colRed)
	styleWikiValid  = lipgloss.NewStyle().Foreground(colAccent).Underline(true)
	styleWikiBroken = lipgloss.NewStyle().Foreground(colRed).Strikethrough(true)
	styleWebLink    = lipgloss.NewStyle().Foreground(colCyan).Underline(true)
	styleLinkFocus  = lipgloss.NewStyle().Foreground(colBg).Background(colAccent).Bold(true)
	styleSearchHit  = lipgloss.NewStyle().Foreground(colBg).Background(colPurple).Bold(true)
)
