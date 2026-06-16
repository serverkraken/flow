package theme

import (
	"strings"

	"charm.land/lipgloss/v2"
)

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

// Active renders s with Sem.Active (Cyan) + Bold — the canonical
// "running / live / in-progress" foreground. Distinct from Info
// (same hex, different role: Info is informational-without-action,
// Active marks a process that is currently happening). Skill
// §Color semantics requires the role-name in code so a palette swap
// that redefines Active without touching Info stays coherent.
func Active(s string, p Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Sem().Active).
		Bold(true).
		Render(s)
}

// Body — default paragraph text. Fg foreground, no extra styling.
func Body(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Fg).Render(s)
}

// Dim — secondary / hint text. FgMuted foreground. Use for footer
// hints, "lädt…" placeholders, scroll-percent indicators.
func Dim(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.FgMuted).Render(s)
}

// Strong — bold body text. Same colour as Body, just emphasised.
func Strong(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(s)
}

// Heading — section / panel title. Bold + Accent (= Blue). Use for
// box titles and screen-level headers.
func Heading(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Sem().Accent).Bold(true).Render(s)
}

// Highlight — purple bold. Use for "this is the active thing" accents
// (status-bar session-name pill, modal title, attached-note marker).
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

// Danger — red bold. Failure / blocking text. Note: error messages in
// body prose use Err (not bold) — Danger is for short status labels
// ("FAIL", "✗ Fehler beim Speichern").
func Danger(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render(s)
}

// Err — non-bold red prose for error-message paragraphs. Same colour
// as Danger but without Bold; Bold on a multi-line error reads as
// shouting. Use Danger for short labels and Err for sentences.
func Err(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Red).Render(s)
}

// Info — cyan, no bold. Use for informational meta — "läuft seit X",
// "scrollen mit ↑/↓".
func Info(s string, p Palette) string {
	return lipgloss.NewStyle().Foreground(p.Cyan).Render(s)
}

// Gap returns a string of n spaces. Use with theme.PadXS / PadSM / PadMD
// instead of inline `"  "` string literals — makes the Skill §Spacing
// "discrete scale, never free integer" rule mechanically enforceable
// and a grep for raw space-strings in render code becomes meaningful.
// n ≤ 0 returns "" so a Gap(maxWidth - usedWidth) can collapse safely.
func Gap(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
