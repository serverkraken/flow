package nodetree

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

func renderView(r *Route, f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}

	if !r.loaded {
		return theme.Dim("  Knoten lädt …", pal)
	}
	if r.err != nil {
		return theme.Err("  Fehler: "+r.err.Error(), pal)
	}
	if r.dialog == dialogDelete {
		return r.confirm.View()
	}
	if r.dialog == dialogMove {
		return r.renderMove(f)
	}

	var b strings.Builder

	// Header: kind filter + optional fuzzy query.
	head := "  Filter: " + kindFilterLabel(r.kind) + "  " +
		lipgloss.NewStyle().Foreground(pal.FgMuted).Render("([ ] Typ · / suchen)")
	b.WriteString(lipgloss.NewStyle().Foreground(pal.FgMuted).Render(head) + "\n")
	if r.filtering || r.query != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.Sem().Accent).Render("/"+r.query) + "\n")
	}
	b.WriteString("\n")

	if len(r.rows) == 0 {
		b.WriteString(theme.Dim("  Keine Knoten.", pal))
		b.WriteString(r.toast.View())
		return b.String()
	}

	selBar := lipgloss.NewStyle().Foreground(pal.Sem().Accent)
	for i, row := range r.rows {
		selected := i == r.cur.Index()
		bar := " "
		if selected {
			bar = selBar.Render(glyphs.AccentBar)
		}
		indent := strings.Repeat("  ", row.Depth)
		glyph := row.Node.Glyph
		if glyph == "" {
			glyph = kindcolor.NodeKindGlyph(row.Node.Kind)
		}
		gStr := lipgloss.NewStyle().Foreground(kindcolor.NodeKindColor(row.Node.Kind, pal)).Render(glyph)
		nameStyle := lipgloss.NewStyle().Foreground(pal.Fg)
		if selected {
			nameStyle = nameStyle.Bold(true)
		}
		kindTag := theme.Dim(kindcolor.NodeKindLabel(row.Node.Kind), pal)
		var status string
		switch row.Node.Status {
		case domain.NodeArchived:
			status = "  " + theme.Dim("[archiviert]", pal)
		case domain.NodePaused:
			status = "  " + theme.Dim("[pausiert]", pal)
		}
		b.WriteString("  " + bar + " " + indent + gStr + " " + nameStyle.Render(row.Node.Name) +
			"  " + kindTag + status + "\n")
	}
	b.WriteString(r.toast.View())
	return b.String()
}
