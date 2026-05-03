package theme

import "github.com/charmbracelet/lipgloss"

// Style builders — pure (string, Palette) → string transforms over
// the 11-field components/theme.Palette (the screen-side palette).
// These replace the bulk of inline lipgloss.NewStyle() calls in
// screens (audit §1.3: 98 occurrences across 8 files) so a palette
// or stylistic change stays one edit, not 98.
//
// The builders are deliberately thin — they cover the seven roles
// that make up most screen text (Dim/Strong/Danger/…). Anything
// needing layout (Width / Padding / Border / JoinHorizontal) still
// calls lipgloss.NewStyle() directly; those genuinely require the
// chained-builder API.
//
// Naming is by *role*, not by colour — so a palette change carries
// through without renaming call-sites. The set mirrors canonical
// theme/builders.go; the difference is the input type (this Palette
// has 11 fields, the canonical one 22).

// Body — default paragraph text. Fg foreground, no extra styling.
func Body(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Fg).Render(s)
}

// Dim — secondary / hint text. Maps onto the palette's Dim field
// (which is canonical FgMuted at the source). Use this for footer
// hints, "lädt…" placeholders, scroll-percent indicators.
func Dim(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Render(s)
}

// Strong — bold body text. Same colour as Body, just emphasised.
func Strong(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(s)
}

// Heading — section / panel title. Bold + Accent. Use for box
// titles and screen-level headers. Card title goes through the card
// component instead.
func Heading(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render(s)
}

// Highlight — purple bold. Use for "Identity" / "this is the active
// thing" type accents (status-bar session-name pill, modal title).
func Highlight(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Purple).Bold(true).Render(s)
}

// Success — green bold. Status / achievement text.
func Success(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Green).Bold(true).Render(s)
}

// Warning — yellow bold. Heads-up / pending text.
func Warning(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Yellow).Bold(true).Render(s)
}

// Danger — red bold. Failure / blocking text. Note: error messages
// in body prose use Err (not bold) — Danger is for short status
// labels ("FAIL", "✗ Fehler beim Speichern").
func Danger(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render(s)
}

// Err — non-bold red prose for error-message paragraphs. Same colour
// as Danger but without Bold — bold would make a multi-line error
// shout. Use Danger for short labels and Err for sentences.
func Err(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Red).Render(s)
}

// Info — cyan, no bold. Use for informational meta — "läuft seit X",
// "scrollen mit ↑/↓".
func Info(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Cyan).Render(s)
}
