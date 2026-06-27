package projects

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// View implements shell.Route.
func (r *FormRoute) View(f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	sem := pal.Sem()
	var b strings.Builder

	// --- error line ---
	if r.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(sem.Danger)
		b.WriteString("  " + errStyle.Render("! "+r.err) + "\n\n")
	}

	labelStyle := lipgloss.NewStyle().Foreground(pal.FgMuted)
	label := func(s string) string { return labelStyle.Render(fmt.Sprintf("  %-13s", s)) }

	// Helper: selector row rendered as "‹ value ›"
	selStr := func(val string) string {
		return lipgloss.NewStyle().Foreground(pal.Fg).Render("‹ " + val + " ›")
	}

	// Focus indicator.
	focusStyle := lipgloss.NewStyle().Foreground(sem.Accent)
	focusBar := func(idx int) string {
		if r.focusIdx == idx {
			return focusStyle.Render("▎")
		}
		return " "
	}

	// Text fields.
	textFields := []struct {
		focus int
		label string
		ti    int
	}{
		{focusName, "Name", 0},
		{focusSlug, "Slug", 1},
		{focusDescription, "Beschreibung", 2},
		{focusUpstream, "Upstream Git", 3},
	}
	for _, f := range textFields {
		b.WriteString(focusBar(f.focus) + label(f.label) + r.inputs[f.ti].View() + "\n")
	}
	b.WriteString("\n")

	// Status selector.
	statusVal := statusLabel(domain.NodeStatus(statusChoices[r.statusIx]))
	b.WriteString(focusBar(focusStatus) + label("Status") + selStr(statusVal) + "\n")

	// Farbe selector: show swatch color via kindcolor.
	colorName := colorChoices[r.colorIx]
	var colorDisplay string
	if colorName == "" {
		colorDisplay = "(keine)"
	} else {
		swatchColor := kindcolor.NodeColor(colorName, pal)
		colorDisplay = lipgloss.NewStyle().Foreground(swatchColor).Render("■") + " " + colorName
	}
	b.WriteString(focusBar(focusColor) + label("Farbe") + selStr(colorDisplay) + "\n")

	// Glyph selector.
	glyphVal := glyphChoices[r.glyphIx]
	if glyphVal == "" {
		glyphVal = "(kein)"
	}
	b.WriteString(focusBar(focusGlyph) + label("Glyph") + selStr(glyphVal) + "\n")

	b.WriteString("\n")

	// Rate fields.
	b.WriteString(focusBar(focusRateAmount) + label("Stundensatz") + r.inputs[4].View() + "\n")
	b.WriteString(focusBar(focusRateCurrency) + label("Währung") + r.inputs[5].View() + "\n")

	b.WriteString("\n")

	// Footer hint.
	hintStyle := lipgloss.NewStyle().Foreground(pal.FgMuted)
	b.WriteString("  " + hintStyle.Render("Enter speichern · Esc abbrechen · ←/→ Auswahl") + "\n")

	return b.String()
}

// KeyHints implements shell.Route.
func (r *FormRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "enter", Desc: "speichern"},
		{Key: "esc", Desc: "abbrechen"},
		{Key: "←/→", Desc: "Auswahl"},
		{Key: "tab", Desc: "Feld"},
	}
}
