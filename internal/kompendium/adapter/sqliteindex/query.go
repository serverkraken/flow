package sqliteindex

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// Search implements ports.Indexer.
func (i *Indexer) Search(ctx context.Context, q domain.SearchQuery) ([]domain.SearchResult, error) {
	query, args := buildSearch(q)
	rows, err := i.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.SearchResult
	for rows.Next() {
		var idStr, title, snippet string
		var score float64
		if err := rows.Scan(&idStr, &title, &snippet, &score); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		out = append(out, domain.SearchResult{
			ID:      domain.ID(idStr),
			Title:   title,
			Snippet: snippet,
			Score:   score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows: %w", err)
	}
	return out, nil
}

// BacklinksOf implements ports.Indexer. Resolves each src_id to its
// title via a LEFT JOIN so the caller doesn't have to do an N+1
// roundtrip back through the note store. Backlinks whose source was
// deleted (no row in notes) surface with an empty title.
func (i *Indexer) BacklinksOf(ctx context.Context, id domain.ID) ([]domain.LinkRef, error) {
	return i.queryLinkRefs(ctx, `
        SELECT l.src_id, COALESCE(n.title, '')
        FROM links l
        LEFT JOIN notes n ON n.id = l.src_id
        WHERE l.dst_id = ?
        ORDER BY l.src_id
    `, id.String())
}

// LinksFrom implements ports.Indexer. Same join shape as BacklinksOf
// for symmetry.
func (i *Indexer) LinksFrom(ctx context.Context, id domain.ID) ([]domain.LinkRef, error) {
	return i.queryLinkRefs(ctx, `
        SELECT l.dst_id, COALESCE(n.title, '')
        FROM links l
        LEFT JOIN notes n ON n.id = l.dst_id
        WHERE l.src_id = ?
        ORDER BY l.dst_id
    `, id.String())
}

func (i *Indexer) queryLinkRefs(ctx context.Context, query string, args ...any) ([]domain.LinkRef, error) {
	rows, err := i.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.LinkRef
	for rows.Next() {
		var idStr, title string
		if err := rows.Scan(&idStr, &title); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, domain.LinkRef{ID: domain.ID(idStr), Title: title})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

func buildSearch(q domain.SearchQuery) (string, []any) {
	var sb strings.Builder
	args := []any{}

	if q.Text != "" {
		sb.WriteString(`SELECT n.id, COALESCE(n.title, ''),
            snippet(notes_fts, 2, '<mark>', '</mark>', '...', 32),
            bm25(notes_fts)
            FROM notes_fts JOIN notes n ON notes_fts.id = n.id
            WHERE notes_fts MATCH ?`)
		args = append(args, escapeFTS5(q.Text))
	} else {
		sb.WriteString(`SELECT n.id, COALESCE(n.title, ''), '', 0.0
            FROM notes n WHERE 1=1`)
	}

	if q.Type != "" {
		sb.WriteString(` AND n.type = ?`)
		args = append(args, string(q.Type))
	}
	if q.Project != "" {
		sb.WriteString(` AND n.project = ?`)
		args = append(args, q.Project)
	}

	switch {
	case q.Order == domain.OrderRecent:
		sb.WriteString(` ORDER BY n.mtime DESC`)
	case q.Text != "":
		sb.WriteString(` ORDER BY bm25(notes_fts) ASC`)
	default:
		sb.WriteString(` ORDER BY n.mtime DESC`)
	}

	if q.Limit > 0 {
		sb.WriteString(` LIMIT ?`)
		args = append(args, q.Limit)
	}
	return sb.String(), args
}

// escapeFTS5 turns a free-form user query into an FTS5-safe MATCH
// expression. Each whitespace-separated token becomes a quoted phrase, so
// inputs like "c++", "foo:bar", `"unterminated`, or `bug(typescript)` —
// all of which would otherwise hit FTS5 syntax errors — match as literal
// text. Multiple tokens are AND-ed implicitly per FTS5 semantics. An
// empty result becomes an empty MATCH which the caller already short-
// circuits in buildSearch.
func escapeFTS5(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		// Inside an FTS5 phrase, " is the only special character, escaped
		// by doubling. Everything else (including operators like *, :, ()
		// and boolean keywords) is treated as literal content.
		f = strings.ReplaceAll(f, `"`, `""`)
		parts = append(parts, `"`+f+`"`)
	}
	return strings.Join(parts, " ")
}
