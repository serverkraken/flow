package nodetree

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/badge"
)

func renderDetailView(r *DetailRoute, f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	var b strings.Builder

	// header: kind glyph + name + kind badge.
	glyph := lipgloss.NewStyle().
		Foreground(kindcolor.NodeKindColor(r.n.Kind, pal)).
		Render(kindcolor.NodeKindGlyph(r.n.Kind))
	name := lipgloss.NewStyle().Foreground(pal.Fg).Bold(true).Render(r.n.Name)
	kb := badge.Render(kindcolor.NodeKindLabel(r.n.Kind), kindcolor.NodeKindColor(r.n.Kind, pal), pal)
	b.WriteString("  " + glyph + " " + name + "  " + kb + "\n\n")

	// breadcrumb (root→leaf) from the leaf→root chain returned by Ancestors.
	if len(r.data.chain) > 0 {
		names := make([]string, 0, len(r.data.chain))
		for i := len(r.data.chain) - 1; i >= 0; i-- {
			names = append(names, r.data.chain[i].Name)
		}
		b.WriteString("  " + theme.Dim(strings.Join(names, " › "), pal) + "\n\n")
	}

	// description.
	if r.n.Description != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.Fg).Render(r.n.Description) + "\n\n")
	}

	// rate — engagement only.
	if r.n.Kind == domain.KindEngagement {
		lbl := lipgloss.NewStyle().Foreground(pal.FgMuted).Render("Satz: ")
		if r.n.Rate != nil {
			b.WriteString("  " + lbl + lipgloss.NewStyle().Foreground(pal.Sem().Success).Render(r.n.Rate.String()) + "\n\n")
		} else {
			b.WriteString("  " + lbl + theme.Dim("kein Satz", pal) + "\n\n")
		}
	}

	// upstream git.
	if r.n.UpstreamGit != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).Render("Git: ") +
			lipgloss.NewStyle().Foreground(pal.Fg).Render(r.n.UpstreamGit) + "\n\n")
	}

	// bindings (read-only).
	if len(r.data.binds) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).
			Render(fmt.Sprintf("Bindings (%d):", len(r.data.binds))) + "\n")
		for _, bd := range r.data.binds {
			b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).
				Render(string(bd.Kind)+": "+bindingTarget(bd)) + "\n")
		}
		b.WriteString("\n")
	}

	// assigned docs count.
	b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).
		Render(fmt.Sprintf("Dokumente (%d):", len(r.data.docs))) + "\n")
	for _, d := range r.data.docs {
		title := d.Title
		if title == "" {
			title = d.Path
		}
		b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).Render(title) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("  " + theme.Dim("e Bearbeiten · q Zurück", pal) + "\n")
	return b.String()
}

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
