package browse

// Modal- und Overlay-Render-Helper. Split aus model.go (Skill
// §No-Monoliths): Modal-Render-Pfade waren tief im 1855-Zeilen-File
// vergraben; getrennt vom List-/Header-Render-Cluster lesen sich die
// confirm-/help-spezifischen Style-Entscheidungen klarer.

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/modal"
)

// renderDeleteModal — Skill §Component vocabulary + §Visual hierarchy:
// Single-Question + Single-Hint statt vierfacher Bestätigungs-Affordance
// (vorher: Headline, Target-ID, Prompt, Key-Pillen, Hint — zu dicht).
// Wording deutsch, kanonisches y/Enter → ja, n/Esc → nein. Frame
// kommt aus components/modal (Kind = Danger → Red DoubleBorder); die
// internen Style-Vars (modalDangerStyle, modalQuestionStyle,
// modalHintStyle) bleiben für die Inhalts-Hierarchie.
func (m Model) renderDeleteModal() string {
	headline := modalDangerStyle.Render(glyphs.Failed + "  Notiz löschen?")
	target := modalQuestionStyle.Render(m.deleteTargetID.String())
	hint := modalHintStyle.Render("y/Enter → ja  ·  n/Esc → nein")
	body := lipgloss.JoinVertical(lipgloss.Center, headline, "", target, "", hint)
	return modal.Render(body, modal.Opts{Kind: modal.KindDanger}, pal)
}

// renderHelpOverlay nutzt components/modal in der Default-Variante
// (Accent-Border) — der Help-Inhalt ist informativ, nicht safe-/danger-
// markiert. Der Inline-Title („Tastenbelegung") bleibt in body, weil
// modal.Opts.Title unter dem Border sitzt und doppelt wäre.
func (m Model) renderHelpOverlay() string {
	title := modalQuestionStyle.Render("Tastenbelegung")
	hForm := help.New()
	hForm.ShowAll = true
	hForm.Width = 70
	hForm.Styles = m.helpUI.Styles
	body := hForm.View(m.keys)
	hint := modalHintStyle.Render("? / Esc → schließen")
	return modal.Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint),
		modal.Opts{}, pal,
	)
}

// frameContent wraps content in the rounded outer frame and explicitly
// pads it to fill the full terminal height. lipgloss.Style.Height does
// not reliably pad bordered styles whose vertical padding is 0, so the
// frame would otherwise stop right after the footer and leave the
// bottom half of the pane bare. Manual padding is the cheap, reliable
// fix.
func frameContent(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return content
	}
	innerW := width - 2
	if innerW <= 0 {
		return content
	}
	targetLines := height - 2 // top + bottom border
	if targetLines > 0 {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines < targetLines {
			content += strings.Repeat("\n", targetLines-contentLines)
		}
	}
	return frameStyle.Width(innerW).Render(content)
}

// overlay places `top` centered over a dotted backdrop. lipgloss in this
// version doesn't expose Layer/Canvas, so a true splice over `base` would
// mean ANSI-aware line surgery — fragile next to glamour-rendered preview
// content. Instead the backdrop uses a subtle dotted fill so the modal
// reads as floating, not as a context-blanking takeover.
func overlay(base, top string, width, height int) string {
	_ = base
	if width <= 0 || height <= 0 {
		return top
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, top,
		lipgloss.WithWhitespaceChars("·"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color(pal.BgChip)))
}
