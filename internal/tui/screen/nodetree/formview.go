package nodetree

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// renderFormView renders the node create/edit form into the given frame.
func renderFormView(r *FormRoute, fr shell.Frame) string {
	pal := fr.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	sem := pal.Sem()
	var b strings.Builder

	// Error line.
	if r.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(sem.Danger)
		b.WriteString("  " + errStyle.Render("! "+r.err) + "\n\n")
	}

	labelStyle := lipgloss.NewStyle().Foreground(pal.FgMuted)
	label := func(s string) string { return labelStyle.Render(fmt.Sprintf("  %-14s", s)) }

	// Selector rendered as "‹ value ›".
	selStr := func(val string) string {
		return lipgloss.NewStyle().Foreground(pal.Fg).Render("‹ " + val + " ›")
	}

	// Focus bar: accent line when focused, space otherwise.
	focusStyle := lipgloss.NewStyle().Foreground(sem.Accent)
	focusBar := func(idx int) string {
		if r.focusIdx == idx {
			return focusStyle.Render("▎")
		}
		return " "
	}

	// Text fields.
	type textField struct {
		focus int
		label string
		ti    int
	}
	textFields := []textField{
		{focusName, "Name", 0},
		{focusSlug, "Slug", 1},
		{focusDescription, "Beschreibung", 2},
		{focusUpstream, "Upstream Git", 3},
	}
	for _, tf := range textFields {
		b.WriteString(focusBar(tf.focus) + label(tf.label) + r.inputs[tf.ti].View() + "\n")
	}
	b.WriteString("\n")

	// Kind selector — create mode only; read-only in edit mode.
	if r.editing != nil {
		kindLabel := kindNodeLabel(r.editing.Kind)
		b.WriteString(" " + label("Art") + kindLabel + "\n")
	} else {
		kindVal := kindNodeLabel(domain.NodeKind(kindChoices[r.kindIx]))
		b.WriteString(focusBar(focusKind) + label("Art") + selStr(kindVal) + "\n")

		// Parent selector — create mode only, hidden for engagement (always root).
		if r.currentKind() != domain.KindEngagement {
			parentVal := "(Wurzel / keine)"
			if r.parentIx > 0 && r.parentIx < len(r.parentLabel) {
				parentVal = r.parentLabel[r.parentIx]
			}
			b.WriteString(focusBar(focusParent) + label("Übergeordnet") + selStr(parentVal) + "\n")
		}
	}

	// Status selector.
	statusVal := nodeStatusLabel(domain.NodeStatus(statusChoices[r.statusIx]))
	b.WriteString(focusBar(focusStatus) + label("Status") + selStr(statusVal) + "\n")

	// Color selector: show swatch + name.
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

	// Rate fields — engagement only.
	if r.currentKind() == domain.KindEngagement {
		b.WriteString("\n")
		b.WriteString(focusBar(focusRateAmount) + label("Stundensatz") + r.inputs[4].View() + "\n")
		b.WriteString(focusBar(focusRateCurrency) + label("Währung") + r.inputs[5].View() + "\n")
	}

	b.WriteString("\n")

	// Footer hint.
	hintStyle := lipgloss.NewStyle().Foreground(pal.FgMuted)
	b.WriteString("  " + hintStyle.Render("tab Feld · ← → Auswahl · enter speichern · esc abbrechen") + "\n")

	return b.String()
}

// View implements shell.Route.
func (r *FormRoute) View(fr shell.Frame) string { return renderFormView(r, fr) }

// KeyHints implements shell.Route.
func (r *FormRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "←/→", Desc: "Auswahl"},
		{Key: "enter", Desc: "speichern"},
		{Key: "esc", Desc: "abbrechen"},
	}
}

// kindNodeLabel returns a human-readable label for a NodeKind.
func kindNodeLabel(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "Engagement"
	case domain.KindVorhaben:
		return "Vorhaben"
	case domain.KindRepo:
		return "Repo"
	case domain.KindBranch:
		return "Branch"
	default:
		return string(k)
	}
}

// nodeStatusLabel returns the German display label for a NodeStatus.
func nodeStatusLabel(s domain.NodeStatus) string {
	switch s {
	case domain.NodeActive:
		return "aktiv"
	case domain.NodePaused:
		return "pausiert"
	case domain.NodeArchived:
		return "archiviert"
	default:
		return string(s)
	}
}
