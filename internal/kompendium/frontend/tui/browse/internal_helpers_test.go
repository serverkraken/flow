package browse

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// White-box tests for the small unexported adapters in browse: the
// usecase → markdown frontmatter / backlink conversion and the
// in-package browseResolver implementation that bridges loaded
// NoteEntry data into a flowports.WikilinkResolver.

func TestBacklinksToMarkdown_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	if got := backlinksToMarkdown(nil); got != nil {
		t.Errorf("nil refs should yield nil, got %+v", got)
	}
	if got := backlinksToMarkdown([]usecase.BacklinkRef{}); got != nil {
		t.Errorf("empty refs should yield nil, got %+v", got)
	}
}

func TestBacklinksToMarkdown_MapsIDAndTitle(t *testing.T) {
	t.Parallel()
	refs := []usecase.BacklinkRef{
		{ID: domain.ID("notes/a"), Title: "A"},
		{ID: domain.ID("notes/b"), Title: ""},
	}
	got := backlinksToMarkdown(refs)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].ID != "notes/a" || got[0].Title != "A" {
		t.Errorf("first mapping: %+v", got[0])
	}
	if got[1].ID != "notes/b" {
		t.Errorf("second mapping: %+v", got[1])
	}
	_ = markdown.BacklinkRef{}
}

func TestFrontmatterToMarkdown_NilReturnsNil(t *testing.T) {
	t.Parallel()
	if got := frontmatterToMarkdown(nil); got != nil {
		t.Errorf("nil fm should yield nil, got %+v", got)
	}
}

func TestFrontmatterToMarkdown_Copies(t *testing.T) {
	t.Parallel()
	fm := &domain.Frontmatter{
		ID:      "notes/x",
		Type:    domain.TypeFree,
		Title:   "Title",
		Project: "github.com/foo/bar",
		Date:    "2026-05-01",
		Tags:    []string{"a", "b"},
	}
	got := frontmatterToMarkdown(fm)
	if got == nil {
		t.Fatalf("non-nil fm should produce non-nil output")
	}
	if got.ID != "notes/x" || got.Title != "Title" || got.Project != "github.com/foo/bar" {
		t.Errorf("conversion: %+v", got)
	}
}

func TestBrowseResolver_Resolve(t *testing.T) {
	t.Parallel()
	r := browseResolver{entries: map[domain.ID]ports.NoteEntry{
		domain.ID("notes/x"): {ID: domain.ID("notes/x"), Meta: domain.Frontmatter{Title: "X"}},
	}}
	uri, title, ok := r.Resolve("notes/x")
	if !ok || uri != "kompendium://note/notes/x" || title != "X" {
		t.Errorf("resolve hit: (%q, %q, %v)", uri, title, ok)
	}
	if _, _, ok := r.Resolve("notes/missing"); ok {
		t.Errorf("missing target should not resolve")
	}
}

// SetPalette is the package-level palette swap. Just verify it doesn't
// panic with theme.Default.
func TestSetPalette_NoPanic(t *testing.T) {
	t.Parallel()
	SetPalette(theme.Default)
	SetPalette(theme.Load())
}

func TestModel_HelpSections_NonEmpty(t *testing.T) {
	t.Parallel()
	sections := Model{}.HelpSections()
	if len(sections) == 0 {
		t.Fatalf("HelpSections should not be empty")
	}
}
