package apistore_test

import (
	"context"
	"sort"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/apistore"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
)

// seedDoc is a helper that puts a markdown document into a FakeDocStore.
// body should already include frontmatter.
func seedDoc(t *testing.T, ds *testutil.FakeDocStore, user, id, rawBody string) {
	t.Helper()
	_, err := ds.Put(user, id+".md", rawBody, "", 0)
	if err != nil {
		t.Fatalf("seedDoc(%q): %v", id, err)
	}
}

func fmBody(id, title, body string) string {
	return "---\nid: " + id + "\ntype: free\ntitle: " + title + "\n---\n" + body + "\n"
}

// TestBacklinksOf verifies that BacklinksOf returns only notes that link TO the
// target and resolves their titles. Three notes: A and B link to C; D does not.
func TestBacklinksOf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	seedDoc(t, ds, testUser, "notes/a", fmBody("notes/a", "Note A", "see [[notes/c]] here"))
	seedDoc(t, ds, testUser, "notes/b", fmBody("notes/b", "Note B", "also [[notes/c]] referenced"))
	seedDoc(t, ds, testUser, "notes/c", fmBody("notes/c", "Note C", "no outbound links"))
	seedDoc(t, ds, testUser, "notes/d", fmBody("notes/d", "Note D", "links to [[notes/a]] only"))

	s := apistore.New(ds, testUser)

	links, err := s.BacklinksOf(ctx, domain.ID("notes/c"))
	if err != nil {
		t.Fatalf("BacklinksOf: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 backlinks, got %d: %+v", len(links), links)
	}

	// Sort for determinism.
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })

	if links[0].ID != "notes/a" || links[0].Title != "Note A" {
		t.Errorf("backlink[0] = %+v, want {notes/a, Note A}", links[0])
	}
	if links[1].ID != "notes/b" || links[1].Title != "Note B" {
		t.Errorf("backlink[1] = %+v, want {notes/b, Note B}", links[1])
	}
}

// TestBacklinksOf_None verifies that a note with no inbound links returns an
// empty slice, not an error.
func TestBacklinksOf_None(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	seedDoc(t, ds, testUser, "notes/solo", fmBody("notes/solo", "Solo", "no links here"))

	s := apistore.New(ds, testUser)
	links, err := s.BacklinksOf(ctx, domain.ID("notes/solo"))
	if err != nil {
		t.Fatalf("BacklinksOf: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 backlinks, got %d", len(links))
	}
}

// TestLinksFrom verifies that LinksFrom returns the outbound links from a note
// with resolved titles, and Title="" for targets not in the corpus.
func TestLinksFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	// notes/src links to notes/target (in corpus) and notes/ghost (not in corpus).
	seedDoc(t, ds, testUser, "notes/src", fmBody("notes/src", "Src", "[[notes/target]] and [[notes/ghost]]"))
	seedDoc(t, ds, testUser, "notes/target", fmBody("notes/target", "Target", "body"))

	s := apistore.New(ds, testUser)

	links, err := s.LinksFrom(ctx, domain.ID("notes/src"))
	if err != nil {
		t.Fatalf("LinksFrom: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %+v", len(links), links)
	}

	// Sort for determinism.
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })

	if links[0].ID != "notes/ghost" || links[0].Title != "" {
		t.Errorf("links[0] = %+v, want {notes/ghost, \"\"}", links[0])
	}
	if links[1].ID != "notes/target" || links[1].Title != "Target" {
		t.Errorf("links[1] = %+v, want {notes/target, Target}", links[1])
	}
}

// TestLinksFrom_Missing verifies that LinksFrom on a non-existent note returns
// nil, nil.
func TestLinksFrom_Missing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	s := apistore.New(ds, testUser)
	links, err := s.LinksFrom(ctx, domain.ID("notes/nonexistent"))
	if err != nil {
		t.Fatalf("LinksFrom: %v", err)
	}
	if links != nil {
		t.Errorf("expected nil slice, got %v", links)
	}
}

// TestBacklinksBidirectional verifies cross-link scenarios: A↔B (mutual links),
// C→A (one-way). BacklinksOf(A) should find B and C; LinksFrom(A) should find B.
func TestBacklinksBidirectional(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	seedDoc(t, ds, testUser, "notes/a", fmBody("notes/a", "A", "[[notes/b]]"))
	seedDoc(t, ds, testUser, "notes/b", fmBody("notes/b", "B", "[[notes/a]]"))
	seedDoc(t, ds, testUser, "notes/c", fmBody("notes/c", "C", "[[notes/a]]"))

	s := apistore.New(ds, testUser)

	// BacklinksOf(A) → B and C
	bl, err := s.BacklinksOf(ctx, domain.ID("notes/a"))
	if err != nil {
		t.Fatalf("BacklinksOf: %v", err)
	}
	if len(bl) != 2 {
		t.Fatalf("BacklinksOf(a): want 2, got %d: %+v", len(bl), bl)
	}

	// LinksFrom(A) → B (only)
	lf, err := s.LinksFrom(ctx, domain.ID("notes/a"))
	if err != nil {
		t.Fatalf("LinksFrom: %v", err)
	}
	if len(lf) != 1 || lf[0].ID != "notes/b" {
		t.Errorf("LinksFrom(a): want [{notes/b, B}], got %+v", lf)
	}
}
