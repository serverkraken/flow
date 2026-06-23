package projects

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// renderView is the projects list View — split into view.go to keep
// route.go under the ~200-line no-monolith threshold.
//
// Layout (from §tui-usability vertical rhythm):
//
//	filter line  ("Filter: aktiv + pausiert  ([ ] wechseln)")
//	blank line
//	rows          (" ▎ ◆ Name                 aktiv" / "   ● …")
//	…
//	blank line
//	empty state   (dim "Keine Projekte.") when shown is empty
func renderView(r *Route, f shell.Frame) string {
	pal := f.Pal
	// Fall back to the palette stored at construction if the frame carries none
	// (e.g. in unit tests that pass a zero Frame). Detect via the zero Bg token.
	if pal.Bg == "" {
		pal = r.pal
	}
	sem := pal.Sem()

	var b strings.Builder

	// --- loading / error states -----------------------------------------
	if !r.loaded {
		b.WriteString(theme.Dim("  Projekte lädt …", pal))
		return b.String()
	}
	if r.err != nil {
		b.WriteString(theme.Err("  Fehler: "+r.err.Error(), pal))
		return b.String()
	}

	// --- filter header ---------------------------------------------------
	filterLabel := r.filter.label()
	filterLine := lipgloss.NewStyle().Foreground(pal.FgMuted).Render(
		"  Filter: " + filterLabel + "  " +
			lipgloss.NewStyle().Foreground(pal.FgMuted).Render("([ ] wechseln)"),
	)
	b.WriteString(filterLine + "\n\n")

	// --- empty state -----------------------------------------------------
	if len(r.shown) == 0 {
		b.WriteString(theme.Dim("  Keine Projekte.", pal))
		return b.String()
	}

	// --- project rows ----------------------------------------------------
	// Prepare shared style objects (mirrors week/route.go cursor treatment).
	selBarStyle := lipgloss.NewStyle().Foreground(sem.Accent)
	selNameStyle := lipgloss.NewStyle().Foreground(pal.Fg).Bold(true)
	defNameStyle := lipgloss.NewStyle().Foreground(pal.Fg)

	for i, p := range r.shown {
		selected := i == r.cur.Index()

		// Selection bar (▎) or blank.
		var selBar string
		if selected {
			selBar = selBarStyle.Render(glyphs.AccentBar)
		} else {
			selBar = " "
		}

		// Identity glyph: project glyph or fallback bullet.
		glyph := p.Glyph
		if glyph == "" {
			glyph = glyphs.Filled
		}
		glyphColor := kindcolor.ProjectColor(p.Color, pal)
		glyphStr := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph)

		// Project name.
		var nameStr string
		if selected {
			nameStr = selNameStyle.Render(p.Name)
		} else {
			nameStr = defNameStyle.Render(p.Name)
		}

		// Status badge — dim secondary text.
		statusStr := theme.Dim(statusLabel(p.Status), pal)

		// Build row: " <selBar> <glyph> <name>   <status>"
		// Left indent (2 cells) + selBar (1) + space (1) + glyph (1) + space (1) + name
		row := "  " + selBar + " " + glyphStr + " " + nameStr + "   " + statusStr
		b.WriteString(row + "\n")
	}

	return b.String()
}

// statusLabel returns the German display label for a project status.
func statusLabel(s domain.ProjectStatus) string {
	switch s {
	case domain.ProjectActive:
		return "aktiv"
	case domain.ProjectPaused:
		return "pausiert"
	case domain.ProjectArchived:
		return "archiviert"
	default:
		return string(s)
	}
}
