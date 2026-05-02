package view

import (
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// frontmatterToMarkdown maps the kompendium domain frontmatter to the
// renderer-facing shape. The markdown package keeps its own
// Frontmatter type so it stays decoupled from the kompendium subtree
// (see CLAUDE-kompendium-plan §K3.B); the adapter lives here because
// the conversion is kompendium-specific.
func frontmatterToMarkdown(fm *domain.Frontmatter) *markdown.Frontmatter {
	if fm == nil {
		return nil
	}
	return &markdown.Frontmatter{
		ID:      fm.ID,
		Type:    markdown.NoteType(fm.Type),
		Project: fm.Project,
		Date:    fm.Date,
		Title:   fm.Title,
		Tags:    fm.Tags,
	}
}

// backlinksToMarkdown maps the use-case-shaped backlink refs to the
// renderer's local BacklinkRef. Same rationale as frontmatterToMarkdown.
func backlinksToMarkdown(refs []usecase.BacklinkRef) []markdown.BacklinkRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]markdown.BacklinkRef, len(refs))
	for i, r := range refs {
		out[i] = markdown.BacklinkRef{ID: r.ID.String(), Title: r.Title}
	}
	return out
}
