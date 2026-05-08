package theme

import "github.com/charmbracelet/lipgloss"

// Style builders — pure (string, Palette) → string transforms. They
// replace the bulk of inline lipgloss.NewStyle() chains in screens and
// components, so a palette or stylistic change is one edit instead of
// scattered hand-rolled chains.
//
// Builders are deliberately thin: they cover the seven-or-so roles
// that account for most rendered text (Body / Dim / Strong / Heading /
// Highlight / Success / Warning / Danger / Err / Info). Anything that
// needs layout (Width / Padding / Border / JoinHorizontal) still goes
// through lipgloss.NewStyle directly — that API genuinely needs the
// chained-builder shape and trying to hide it under a builder costs
// more than it saves.
//
// Naming is by *role*, not by colour, so a palette change carries
// through without renaming any call-site. Roles map onto canonical
// Sem() aliases where the mapping is fixed (Heading → Sem.Accent,
// Success → Sem.Success, …); raw hues are used only where there is
// no semantic alias (Highlight = Purple, Info = Cyan).

// Body — default paragraph text. Fg foreground, no extra styling.
func Body(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Render(s)
}

// Dim — secondary / hint text. FgMuted foreground. Use for footer
// hints, "lädt…" placeholders, scroll-percent indicators.
func Dim(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted)).Render(s)
}

// Strong — bold body text. Same colour as Body, just emphasised.
func Strong(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Bold(true).Render(s)
}

// Heading — section / panel title. Bold + Accent (= Blue). Use for
// box titles and screen-level headers.
func Heading(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Accent)).Bold(true).Render(s)
}

// Highlight — purple bold. Use for "this is the active thing" accents
// (status-bar session-name pill, modal title, attached-note marker).
func Highlight(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Purple)).Bold(true).Render(s)
}

// Success — green bold. Status / achievement text.
func Success(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)).Bold(true).Render(s)
}

// Warning — yellow bold. Heads-up / pending text.
func Warning(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Yellow)).Bold(true).Render(s)
}

// Danger — red bold. Failure / blocking text. Note: error messages in
// body prose use Err (not bold) — Danger is for short status labels
// ("FAIL", "✗ Fehler beim Speichern").
func Danger(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Red)).Bold(true).Render(s)
}

// Err — non-bold red prose for error-message paragraphs. Same colour
// as Danger but without Bold; Bold on a multi-line error reads as
// shouting. Use Danger for short labels and Err for sentences.
func Err(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Red)).Render(s)
}

// Info — cyan, no bold. Use for informational meta — "läuft seit X",
// "scrollen mit ↑/↓".
func Info(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Render(s)
}
