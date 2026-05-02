package browse

import (
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// frontmatterToMarkdown maps the kompendium domain frontmatter to the
// renderer-facing shape. The shared markdown package keeps its own
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
