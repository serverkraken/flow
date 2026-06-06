package handlers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/format"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// buildNotesIndexRows turns NoteEntry slice into IndexRow slice and
// applies the optional substring query against title + tags +
// frontmatter project. Returns the filtered rows and the unfiltered
// total so the empty-state branching has both.
func buildNotesIndexRows(
	ctx context.Context,
	d NotesDeps,
	entries []ports.NoteEntry,
	query string,
	clock flowports.Clock,
) ([]notestmpl.IndexRow, int) {
	now := time.Now()
	if clock != nil {
		now = clock.Now()
	}
	rows := make([]notestmpl.IndexRow, 0, len(entries))
	lcQuery := strings.ToLower(query)
	for _, e := range entries {
		row := buildIndexRow(ctx, d, e, now)
		if !rowMatchesQuery(row, lcQuery, e.Meta) {
			continue
		}
		rows = append(rows, row)
	}
	return rows, len(rows)
}

// buildIndexRow loads just enough of a note to populate one list row.
// Body is fetched lazily for the preview so a notebook of 1k+ notes
// doesn't pay full-body I/O for a single index render. With a small
// notebook (M6 reality), the full Get is cheap; the deferred-load
// shape is here for Phase 2 when notebooks may grow.
func buildIndexRow(ctx context.Context, d NotesDeps, e ports.NoteEntry, now time.Time) notestmpl.IndexRow {
	row := notestmpl.IndexRow{
		ID:      e.ID.String(),
		Type:    notestmpl.GermanTypeLabel(e.Meta.Type),
		Project: e.Meta.Project,
		When:    format.HumanRelativeTime(e.Mtime, now),
		Tags:    append([]string(nil), e.Meta.Tags...),
		Href:    "/notes/" + e.ID.String(),
	}
	// Body fetch — soft-fail: a Get error degrades to title-only
	// rendering rather than dropping the row entirely. The frontmatter
	// metadata already loaded is enough to make the row useful.
	if d.Store != nil {
		if note, err := d.Store.Get(ctx, e.ID); err == nil {
			row.Title = notestmpl.TitleOf(e.Meta.Title, note.Body, e.ID.String())
			row.Preview = notestmpl.BuildPreview(note.Body)
		} else {
			row.Title = notestmpl.TitleOf(e.Meta.Title, nil, e.ID.String())
		}
	} else {
		row.Title = notestmpl.TitleOf(e.Meta.Title, nil, e.ID.String())
	}
	return row
}

// rowMatchesQuery is the substring-match used for the ?q= filter. M6
// keeps it simple: case-insensitive substring across title, tags,
// project, and ID. M7 may grow it to full-text — for now a plain
// strings.Contains is faster than spinning up the sqliteindex.
func rowMatchesQuery(row notestmpl.IndexRow, lcQuery string, meta domain.Frontmatter) bool {
	if lcQuery == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{
		row.Title,
		row.ID,
		row.Project,
		strings.Join(row.Tags, " "),
		strings.Join(meta.Tags, " "),
		meta.Title,
	}, " "))
	return strings.Contains(hay, lcQuery)
}

// buildNotesViewVM resolves a Note into the view-model the template
// renders. Errors from the markdown renderer degrade to an empty body
// rather than 500 — the rail metadata still renders so the user can
// at least navigate.
func buildNotesViewVM(d NotesDeps, note domain.Note) (notestmpl.ViewVM, error) {
	html, err := d.Markdown.Render(note.Body)
	if err != nil {
		// Don't return — degrade to empty body, log at the caller.
		// We still want the rail + header to render.
		html = ""
	}
	vm := notestmpl.ViewVM{
		ID:           note.ID.String(),
		Title:        notestmpl.TitleOf(note.Meta.Title, note.Body, note.ID.String()),
		Path:         note.ID.Path(),
		TypeLabel:    notestmpl.GermanTypeLabel(note.Meta.Type),
		Tags:         append([]string(nil), note.Meta.Tags...),
		HTML:         html,
		CreatedLabel: formatCreatedLabel(note.Meta),
		ModifiedLabel: formatModifiedLabel(d, note),
		SyncLabel:    "lokal · phase 2",
		BreadcrumbHrefs: notestmpl.Breadcrumb{
			NotesHref: "/notes",
			TypeHref:  breadcrumbTypeHref(note.Meta.Type),
		},
	}
	for _, h := range d.Markdown.Headings(note.Body) {
		vm.Headings = append(vm.Headings, notestmpl.HeadingItem{
			Level:  h.Level,
			Text:   h.Text,
			Anchor: h.Anchor,
		})
	}
	return vm, nil
}

// formatCreatedLabel prefers the frontmatter `date` (set by kompendium
// daily / project create flows) and falls back to "—" so the rail
// always renders something.
func formatCreatedLabel(meta domain.Frontmatter) string {
	if meta.Date != "" {
		return meta.Date
	}
	return "—"
}

// formatModifiedLabel reads the note file's mtime via the store's
// Path. Skipped (returns the relative date or "—") when mtime can't
// be read so the rail row stays clean.
func formatModifiedLabel(d NotesDeps, note domain.Note) string {
	if d.Store == nil {
		return "—"
	}
	// We don't have direct mtime on Note; the store List operation
	// reads it, but a single Get doesn't carry it. Re-stat the file
	// via the Path helper — cheap, and only fires on the single-note
	// render path.
	info, err := os.Stat(d.Store.Path(note.ID))
	if err != nil {
		return "—"
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}
	return format.HumanRelativeTime(info.ModTime(), now)
}

// breadcrumbTypeHref maps a NoteType to the index sub-tab URL.
func breadcrumbTypeHref(t domain.NoteType) string {
	switch t {
	case domain.TypeDaily:
		return "/notes?type=daily"
	case domain.TypeProject:
		return "/notes?type=project"
	case domain.TypeFree:
		return "/notes?type=frei"
	default:
		return "/notes?type=alle"
	}
}

