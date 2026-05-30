package browse

// Modal- und Overlay-Render-Helper. Split aus model.go (Skill
// §No-Monoliths): Modal-Render-Pfade waren tief im 1855-Zeilen-File
// vergraben; getrennt vom List-/Header-Render-Cluster lesen sich die
// confirm-/help-spezifischen Style-Entscheidungen klarer.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	flowhelp "github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/modal"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// renderDeleteModal — Skill §Component vocabulary + §Visual hierarchy:
// Single-Question + Single-Hint statt vierfacher Bestätigungs-Affordance
// (vorher: Headline, Target-ID, Prompt, Key-Pillen, Hint — zu dicht).
// Wording deutsch, Hint via uistrings.HintConfirm (A11y-6 bracketed
// default action, drift-frei mit confirm.Model.View und allen anderen
// destructive confirms). Frame kommt aus components/modal (Kind =
// Danger → Red DoubleBorder); die internen Styles (modalDanger,
// modalQuestion, modalHint im browseStyles-Cache) bleiben für die
// Inhalts-Hierarchie.
func (m Model) renderDeleteModal() string {
	headline := m.styles.modalDanger.Render(glyphs.Failed + "  Notiz löschen?")
	target := m.styles.modalQuestion.Render(m.deleteTargetID.String())
	hint := m.styles.modalHint.Render(uistrings.HintConfirm)
	body := lipgloss.JoinVertical(lipgloss.Center, headline, "", target, "", hint)
	return modal.Render(body, modal.Opts{Kind: modal.KindDanger}, m.styles.pal)
}

// renderHelpOverlay renders the standalone `?` overlay through the
// canonical components/help.Render (titlebox + accent section titles +
// fg-key/dim-desc rows), so it matches the sidekick's aggregated `?`
// overlay instead of the old bubbles/help flat-column look. Sections
// come from helpSections() with the screen-identifying "Notizen · "
// prefix stripped — inside this overlay the kompendium context is
// implicit. A trailing all-dim hint strip (m.styles.footer, the
// package-local FgMuted style — kompendium-frontend darf statusbar
// nicht importieren, depguard) mirrors the sidekick footer.
func (m Model) renderHelpOverlay() string {
	sections := helpSections()
	for i := range sections {
		sections[i].Title = strings.TrimPrefix(sections[i].Title, "Notizen · ")
	}
	box := flowhelp.Render("Tastenbelegung", sections, theme.KeyHintWidth, 70, m.styles.pal)
	return box + "\n" + m.styles.footer.Render("? / Esc → schließen")
}

// frameContent wraps content in the rounded outer frame and explicitly
// pads it to fill the full terminal height. lipgloss.Style.Height does
// not reliably pad bordered styles whose vertical padding is 0, so the
// frame would otherwise stop right after the footer and leave the
// bottom half of the pane bare. Manual padding is the cheap, reliable
// fix. Method on Model so the frame style is read from the per-Model
// browseStyles cache (post-Phase-6: no package-level vars).
func (m Model) frameContent(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return content
	}
	if width-2 <= 0 {
		return content
	}
	targetLines := height - 2 // top + bottom border
	if targetLines > 0 {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines < targetLines {
			content += strings.Repeat("\n", targetLines-contentLines)
		}
	}
	// Lipgloss v2's Width(n) is the OUTER total (border + padding +
	// content), unlike v1 where Width was content-only. Pass the full
	// terminal width so the rounded frame's outer edges sit flush with
	// the screen edges; content is already sized to width-2 by the
	// per-section renderers.
	return m.styles.frame.Width(width).Render(content)
}

// overlay places `top` centered over a dotted backdrop. lipgloss in this
// version doesn't expose Layer/Canvas, so a true splice over `base` would
// mean ANSI-aware line surgery — fragile next to glamour-rendered preview
// content. Instead the backdrop uses a subtle dotted fill so the modal
// reads as floating, not as a context-blanking takeover. Method on Model
// so the backdrop colour comes from the per-Model palette (post-Phase-6).
func (m Model) overlay(base, top string, width, height int) string {
	_ = base
	if width <= 0 || height <= 0 {
		return top
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, top,
		lipgloss.WithWhitespaceChars("·"),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(m.styles.pal.BgChip)))
}
