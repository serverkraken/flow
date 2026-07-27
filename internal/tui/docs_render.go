package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	tuistrings "github.com/serverkraken/flow/internal/tui/ui/strings"
)

// docCounts tallies documents per type.
func docCounts(docs []domain.Document) map[domain.DocumentType]int {
	m := make(map[domain.DocumentType]int, 4)
	for _, d := range docs {
		m[d.Type]++
	}
	return m
}

// dateCell is the date column for a row: the daily Date, else UpdatedAt.
func dateCell(d domain.Document) string {
	if d.Date != nil {
		return d.Date.Format("2006-01-02")
	}
	return d.UpdatedAt.Format("2006-01-02")
}

// applyProjectFilter keeps project-less docs (daily/free) plus docs of the
// selected project. projID == "" returns docs unchanged.
func applyProjectFilter(docs []domain.Document, projID string) []domain.Document {
	if projID == "" {
		return docs
	}
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.NodeID == nil || *d.NodeID == projID {
			out = append(out, d)
		}
	}
	return out
}

// projRowLabel is the row's primary label: "slug · title" for project docs,
// else the document path.
func projRowLabel(d domain.Document, projByID map[string]domain.Node) string {
	if d.Type == domain.DocProject && d.NodeID != nil {
		if p, ok := projByID[*d.NodeID]; ok {
			return p.Slug + " · " + d.Title
		}
	}
	return d.Path
}

// docExcerpt collapses whitespace and word-wraps body to width, capped at
// maxLines (the last line ends with … when content is truncated).
func docExcerpt(body string, width, maxLines int) []string {
	if width < 1 || maxLines < 1 {
		return nil
	}
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range fields {
		cand := w
		if cur != "" {
			cand = cur + " " + w
		}
		if lipgloss.Width(cand) > width && cur != "" {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = cand
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = tuistrings.Truncate(lines[maxLines-1]+" …", width)
	}
	return lines
}
