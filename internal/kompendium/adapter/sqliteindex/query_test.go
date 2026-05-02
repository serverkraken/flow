package sqliteindex_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestSearch_FilterByType(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "kompendium body"), unix(2))
	_ = idx.Upsert(ctx, makeNoteWithType(t, "projects/foo/2026-04-25", domain.TypeProject, "github.com/foo", "kompendium body"), unix(1))

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "kompendium", Type: domain.TypeDaily})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "daily/2026-04-25" {
		t.Errorf("filter by type=daily failed, got %+v", got)
	}
}

func TestSearch_FilterByProject(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNoteWithType(t, "projects/foo/2026-04-25", domain.TypeProject, "github.com/foo", "alpha"), unix(2))
	_ = idx.Upsert(ctx, makeNoteWithType(t, "projects/bar/2026-04-25", domain.TypeProject, "github.com/bar", "alpha"), unix(1))

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "alpha", Project: "github.com/foo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "projects/foo/2026-04-25" {
		t.Errorf("filter by project failed, got %+v", got)
	}
}

func TestSearch_NoTextWithFilters(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-22", "x"), unix(1))
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "x"), unix(3))
	_ = idx.Upsert(ctx, makeNoteWithType(t, "notes/free", domain.TypeFree, "", "x"), unix(2))

	got, err := idx.Search(ctx, domain.SearchQuery{Type: domain.TypeDaily})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 daily notes, got %d", len(got))
	}
	// Default order without text: mtime DESC.
	if got[0].ID != "daily/2026-04-25" {
		t.Errorf("expected newest first, got %+v", got)
	}
}

func TestSearch_OrderRecent(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-22", "shared"), unix(1))
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "shared"), unix(3))
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-23", "shared"), unix(2))

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "shared", Order: domain.OrderRecent})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	if got[0].ID != "daily/2026-04-25" || got[2].ID != "daily/2026-04-22" {
		t.Errorf("OrderRecent did not sort by mtime DESC: %+v", got)
	}
}

func TestSearch_Limit(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		id := domain.ID("daily/2026-04-2" + string(rune('1'+i)))
		_ = idx.Upsert(ctx, makeNoteAtID(t, id, "alpha"), unix(int64(i)))
	}

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "alpha", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results (limit), got %d", len(got))
	}
}

// TestSearch_HostileQuerySafe covers inputs that would crash the raw
// FTS5 parser without escaping: an unterminated quote, embedded operator
// keywords, language-y punctuation. The escaper wraps each token in a
// phrase, so these all resolve to substring searches and either match
// literally or return no results — never a syntax error.
func TestSearch_HostileQuerySafe(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", `c++ tooling and bug(typescript) "quoted" foo:bar`), unix(1))

	cases := []string{
		`"unterminated`,   // single dangling quote — old code: syntax error
		`c++`,             // operator-y punctuation — old code: syntax error
		`foo:bar`,         // FTS5 column qualifier — old code: "no such column"
		`bug(typescript)`, // grouping parens — old code: syntax error
		`"quoted"`,        // valid phrase — must still match
		`AND OR NOT`,      // keywords — old code: syntax error
		`*`,               // bare operator — old code: syntax error
		`weird ""inner""`, // embedded doubled quotes
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			if _, err := idx.Search(ctx, domain.SearchQuery{Text: q}); err != nil {
				t.Errorf("Search(%q) unexpected error: %v", q, err)
			}
		})
	}
}

func TestSearch_QuotedPhraseStillMatches(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "kompendium architecture review"), unix(1))

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "architecture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 hit for 'architecture', got %d", len(got))
	}
}

// TestSearch_UnicodeFoldsDiacritics covers the unicode61 tokenizer
// upgrade: searching `garten` must hit a body with `Gärten`, and vice
// versa, so German notes don't slip through full-text search just
// because of an umlaut. With the default `simple` tokenizer this would
// require an exact match.
func TestSearch_UnicodeFoldsDiacritics(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()
	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "Wir haben die Gärten besucht."), unix(1))

	for _, q := range []string{"Gärten", "garten", "GÄRTEN", "gärten"} {
		got, err := idx.Search(ctx, domain.SearchQuery{Text: q})
		if err != nil {
			t.Errorf("Search(%q): %v", q, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("Search(%q) expected 1 hit, got %d", q, len(got))
		}
	}
}

func TestBacklinksAndLinksFrom(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	src := makeNoteAtID(t, "daily/2026-04-25", "see [[notes/setup]] and [[projects/foo]]")
	target1 := makeNoteAtID(t, "notes/setup", "")
	target2 := makeNoteAtID(t, "projects/foo", "")
	_ = idx.Upsert(ctx, src, unix(3))
	_ = idx.Upsert(ctx, target1, unix(2))
	_ = idx.Upsert(ctx, target2, unix(1))

	links, err := idx.LinksFrom(ctx, "daily/2026-04-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].ID != "notes/setup" || links[1].ID != "projects/foo" {
		t.Errorf("LinksFrom IDs unexpected: %+v", links)
	}
	// Title is populated from the join — makeNoteAtID sets a synthetic
	// "title for <id>" frontmatter, which the indexer must surface so
	// RenderBacklinks doesn't have to do an N+1 store lookup.
	for _, l := range links {
		if l.Title == "" {
			t.Errorf("LinksFrom: title not populated by indexer join: %+v", l)
		}
	}

	back, err := idx.BacklinksOf(ctx, "notes/setup")
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0].ID != "daily/2026-04-25" {
		t.Errorf("BacklinksOf got %+v, want one entry for daily/2026-04-25", back)
	}
	if back[0].Title == "" {
		t.Errorf("BacklinksOf: title not populated: %+v", back[0])
	}
}
