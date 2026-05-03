package theme

import "github.com/charmbracelet/lipgloss"

// Style builders — pure (Palette, string) -> string transforms that
// replace the ~70 inline lipgloss.NewStyle() calls scattered across
// screens. Use these by default; reach for NewStyle directly only when
// the layout (Width / PlaceHorizontal / JoinVertical) genuinely can't
// be expressed with a builder.
//
// Naming is by *role* (Heading1, Body, Dim, Code, Success), not by
// color, so a palette change carries through without renaming call
// sites.

// Heading1 — top-level title. Bold + Accent.
func Heading1(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Accent)).Bold(true).Render(s)
}

// Heading2 — section title. Bold + Highlight.
func Heading2(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Highlight)).Bold(true).Render(s)
}

// Heading3 — sub-section. Bold + Active.
func Heading3(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Active)).Bold(true).Render(s)
}

// Body — default paragraph. Fg only.
func Body(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Render(s)
}

// Dim — secondary text (FgDim).
func Dim(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgDim)).Render(s)
}

// Muted — hint / meta. FgMuted; never load-bearing content (A11y-3).
func Muted(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted)).Render(s)
}

// Code — inline code. Green on BgCode panel.
func Code(p Palette, s string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Green)).
		Background(lipgloss.Color(p.BgCode)).
		Render(s)
}

// Strong — bold body text.
func Strong(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Bold(true).Render(s)
}

// Emph — italic body text.
func Emph(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Italic(true).Render(s)
}

// Success — bold green status text.
func Success(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Success)).Bold(true).Render(s)
}

// Warning — bold yellow status text.
func Warning(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Warning)).Bold(true).Render(s)
}

// Danger — bold red status text.
func Danger(p Palette, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Danger)).Bold(true).Render(s)
}
