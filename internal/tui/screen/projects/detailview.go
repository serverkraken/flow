package projects

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/markdown"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// renderDetailView renders the full cockpit view for a project.
// Section order mirrors the WebUI cockpit:
//
//	header → description → Git → Worktime → Worktrees → Dokumente → Bindings → actions hint
func renderDetailView(r *DetailRoute, f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	sem := pal.Sem()

	var b strings.Builder

	// --- header: glyph + name + status badge ---
	glyph := r.p.Glyph
	if glyph == "" {
		glyph = "◆"
	}
	glyphColor := kindcolor.NodeColor(r.p.Color, pal)
	glyphStr := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph)
	nameStr := lipgloss.NewStyle().Foreground(pal.Fg).Bold(true).Render(r.p.Name)
	statusStr := lipgloss.NewStyle().
		Foreground(statusSemColor(r.p.Status, pal)).
		Render("[" + statusLabel(r.p.Status) + "]")
	b.WriteString("  " + glyphStr + " " + nameStr + "  " + statusStr + "\n\n")

	// --- description (markdown rendered) ---
	if r.p.Description != "" {
		width := f.Width - 4
		if width < 20 {
			width = 20
		}
		rendered, err := markdown.Render(r.p.Description, width, markdown.WithPalette(pal))
		if err != nil || rendered == "" {
			// Fallback: show raw text (strip leading # for headings).
			rendered = r.p.Description
		}
		// Indent each line by 2 spaces.
		for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}

	// --- Git upstream ---
	if r.p.UpstreamGit != "" {
		label := lipgloss.NewStyle().Foreground(pal.FgMuted).Render("Git: ")
		val := lipgloss.NewStyle().Foreground(pal.Fg).Render(r.p.UpstreamGit)
		b.WriteString("  " + label + val + "\n\n")
	}

	// --- Worktime aggregate ---
	agg := r.data.agg
	if agg.Total > 0 || r.data.p.ID != "" {
		b.WriteString(renderWorktimeSection(agg, pal, sem))
	}

	// --- Worktrees (read-only) ---
	b.WriteString(renderWorktreeSection(r, pal))

	// --- Dokumente ---
	if len(r.data.docs) > 0 {
		b.WriteString(renderDocsSection(r, pal))
	}

	// --- Bindings ---
	if len(r.data.binds) > 0 {
		b.WriteString(renderBindingsSection(r, pal))
	}

	// --- actions hint ---
	hintStyle := lipgloss.NewStyle().Foreground(pal.FgMuted)
	b.WriteString("  " + hintStyle.Render("e Bearbeiten  p Pausieren  r Fortsetzen  a Archivieren  q Zurück") + "\n")

	return b.String()
}

func renderWorktimeSection(agg worktimeAgg, pal theme.Palette, sem theme.Semantic) string {
	var b strings.Builder
	label := lipgloss.NewStyle().Foreground(pal.FgMuted).Render
	val := lipgloss.NewStyle().Foreground(pal.Fg).Bold(true).Render

	totalStr := formatDurDetail(agg.Total)
	weekStr := formatDurDetail(agg.Week)
	monthStr := formatDurDetail(agg.Month)

	line := "  " + label("Arbeitszeit: ") +
		"Σ " + val(totalStr) + "  " +
		label("Woche: ") + val(weekStr) + "  " +
		label("Monat: ") + val(monthStr)
	if agg.Earnings != "" {
		line += "  " + lipgloss.NewStyle().Foreground(sem.Success).Render(agg.Earnings)
	}
	b.WriteString(line + "\n\n")
	return b.String()
}

func renderWorktreeSection(r *DetailRoute, pal theme.Palette) string {
	var b strings.Builder
	label := lipgloss.NewStyle().Foreground(pal.FgMuted).Render

	b.WriteString("  " + label("Worktrees:") + "\n")
	if r.data.root == "" {
		b.WriteString("    " + theme.Dim("nicht ausgecheckt auf diesem PC", pal) + "\n")
	} else {
		for _, wt := range r.data.wts {
			branch := wt.Branch
			if branch == "" {
				branch = wt.HeadShort
			}
			prefix := "    "
			if wt.IsMain {
				prefix = "  * "
			}
			dirty := ""
			if wt.Dirty {
				dirty = " *"
			}
			line := prefix + branch + dirty + "  " + wt.Path
			b.WriteString(lipgloss.NewStyle().Foreground(pal.Fg).Render(line) + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderDocsSection(r *DetailRoute, pal theme.Palette) string {
	var b strings.Builder
	label := lipgloss.NewStyle().Foreground(pal.FgMuted).Render

	b.WriteString("  " + label(fmt.Sprintf("Dokumente (%d):", len(r.data.docs))) + "\n")
	for _, doc := range r.data.docs {
		title := doc.Title
		if title == "" {
			title = doc.Path
		}
		b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).Render(title) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func renderBindingsSection(r *DetailRoute, pal theme.Palette) string {
	var b strings.Builder
	label := lipgloss.NewStyle().Foreground(pal.FgMuted).Render

	b.WriteString("  " + label(fmt.Sprintf("Bindings (%d):", len(r.data.binds))) + "\n")
	for _, bd := range r.data.binds {
		target := bindingTarget(bd)
		line := string(bd.Kind) + ": " + target
		b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).Render(line) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// bindingTarget mirrors the WebUI bindingTarget helper: remote bindings show
// the RemoteSlug, path bindings show the Path.
func bindingTarget(b domain.ProjectBinding) string {
	switch b.Kind {
	case domain.BindingRemote:
		return b.RemoteSlug
	case domain.BindingPath:
		return b.Path
	default:
		return string(b.Kind)
	}
}

// statusSemColor returns a semantic color for the project status badge.
func statusSemColor(s domain.NodeStatus, pal theme.Palette) theme.Color {
	sem := pal.Sem()
	switch s {
	case domain.NodeActive:
		return sem.Success
	case domain.NodePaused:
		return sem.Warning
	case domain.NodeArchived:
		return pal.FgMuted
	default:
		return pal.FgMuted
	}
}

// formatDurDetail formats a duration as "Xh Ym" (matches the worktime package format).
func formatDurDetail(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}
