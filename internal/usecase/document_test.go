package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateDocument_FreeAndDaily(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	uc := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}

	// free doc
	free, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/architecture", Title: "Arch",
	})
	if err != nil {
		t.Fatalf("free create: %v", err)
	}
	if free.ID == "" {
		t.Error("ID must be stamped")
	}
	if free.OwnerID != "u1" {
		t.Errorf("OwnerID = %q, want u1", free.OwnerID)
	}
	if free.CreatedAt.IsZero() || free.UpdatedAt.IsZero() {
		t.Error("timestamps must be stamped")
	}

	// daily doc — path derived, Date set
	daily, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{Type: domain.DocDaily})
	if err != nil {
		t.Fatalf("daily create: %v", err)
	}
	if daily.Path != "daily/2026-06-15" {
		t.Errorf("daily Path = %q, want daily/2026-06-15", daily.Path)
	}
	if daily.Date == nil {
		t.Error("daily Date must be set")
	}

	// project doc without nodeID — must fail Validate
	_, err = uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocProject, Path: "docs/proj",
	})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Errorf("project without nodeID: want ErrInvalidDocument, got %v", err)
	}
}

func TestCreateDocument_Persisted(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	get := usecase.GetDocument{Docs: docs}

	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/notes", Title: "Notes",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := get.Execute(ctx, "u1", created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID || got.Title != "Notes" {
		t.Errorf("get returned unexpected doc: %+v", got)
	}
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	t0 := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	clk := testutil.FakeClock{T: t0}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/design", Title: "Old title",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	clk.T = t1
	update := usecase.UpdateDocument{Docs: docs, Clock: clk}
	updated, err := update.Execute(ctx, "u1", created.ID, usecase.UpdateDocumentInput{
		Title: "New title", Body: "new body",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "New title" {
		t.Errorf("Title = %q, want New title", updated.Title)
	}
	if updated.Body != "new body" {
		t.Errorf("Body = %q, want new body", updated.Body)
	}
	if !updated.UpdatedAt.Equal(t1) {
		t.Errorf("UpdatedAt = %v, want %v", updated.UpdatedAt, t1)
	}
}

func TestUpdateDocument_NotFound(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.UpdateDocument{Docs: docs, Clock: clk}

	_, err := uc.Execute(ctx, "u1", "nonexistent", usecase.UpdateDocumentInput{Title: "X"})
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("want ErrDocumentNotFound, got %v", err)
	}
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	del := usecase.DeleteDocument{Docs: docs}
	get := usecase.GetDocument{Docs: docs}

	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/temp", Title: "Temp",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := del.Execute(ctx, "u1", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = get.Execute(ctx, "u1", created.ID)
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("after delete: want ErrDocumentNotFound, got %v", err)
	}
}

func TestCreateDocument_WritesLinks(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.CreateDocument{
		Docs:  store,
		IDs:   ids,
		Clock: clk,
	}
	created, err := uc.Execute(ctx, "o", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "src", Title: "Src", Body: "see [[dest]] and [[dest]]",
	})
	if err != nil {
		t.Fatal(err)
	}
	// FakeIDGen returns "id-1" for the first call
	if created.ID != "id-1" {
		t.Fatalf("expected created.ID = id-1, got %q", created.ID)
	}
	_, _ = store.Create(ctx, domain.Document{
		ID: "doc-2", OwnerID: "o", Type: domain.DocFree, Path: "dest", Title: "Dest",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	refs, err := (usecase.Backlinks{Docs: store}).Execute(ctx, "o", "doc-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "id-1" {
		t.Fatalf("expected src (id-1) as the only backlink of dest, got %v", refs)
	}
}

func TestListDocuments_OwnerScoped(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	list := usecase.ListDocuments{Docs: docs}

	// u1 creates two docs; u2 creates one
	if _, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/a", Title: "A",
	}); err != nil {
		t.Fatalf("create u1/a: %v", err)
	}
	if _, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/b", Title: "B",
	}); err != nil {
		t.Fatalf("create u1/b: %v", err)
	}
	if _, err := create.Execute(ctx, "u2", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/c", Title: "C",
	}); err != nil {
		t.Fatalf("create u2/c: %v", err)
	}

	u1Docs, err := list.Execute(ctx, "u1", nil, nil)
	if err != nil {
		t.Fatalf("list u1: %v", err)
	}
	if len(u1Docs) != 2 {
		t.Errorf("u1 list: got %d docs, want 2", len(u1Docs))
	}
	for _, d := range u1Docs {
		if d.OwnerID != "u1" {
			t.Errorf("list returned doc with OwnerID=%q, want u1", d.OwnerID)
		}
	}

	u2Docs, err := list.Execute(ctx, "u2", nil, nil)
	if err != nil {
		t.Fatalf("list u2: %v", err)
	}
	if len(u2Docs) != 1 {
		t.Errorf("u2 list: got %d docs, want 1", len(u2Docs))
	}
}

func TestListTags_RegistryScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"go", "tui"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"deep"})

	uc := usecase.ListTags{Tags: ts}
	docType := domain.TaggableDocument
	got, err := uc.Execute(ctx, "u1", domain.TagScope{Type: &docType})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Tag != "go" || got[0].Count != 2 {
		t.Fatalf("doc-scoped ListTags want go(2),tui(1), got %+v", got)
	}
}

// TestCreateDocument_ExplicitTagsOverrideFrontmatter verifies that explicit tags
// in the input are used; body frontmatter is preserved verbatim but no longer
// used as a tag source after B2.
func TestCreateDocument_ExplicitTagsOverrideFrontmatter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	got, err := uc.Execute(context.Background(), "u", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "note", Title: "Note",
		Body: "---\ntags: [ignored]\n---\nhello",
		Tags: []string{"go", "tui"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"go", "tui"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %#v, want %#v", got.Tags, want)
	}
	if !strings.HasPrefix(got.Body, "---\n") {
		t.Fatal("body must keep frontmatter verbatim")
	}
}

// TestCreateDocument_NoTagsWhenNilInput verifies that nil Tags input results in
// no tags being set (frontmatter fallback removed in B2).
func TestCreateDocument_NoTagsWhenNilInput(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	got, err := uc.Execute(context.Background(), "u", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "note2", Title: "Note2",
		Body: "---\ntags: [ignored]\n---\nhello",
		// Tags: nil — no explicit tags
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("want no tags when input is nil, got %v", got.Tags)
	}
}

// TestUpdateDocument_ExplicitTagsParam verifies that in.Tags != nil updates tags;
// nil in.Tags leaves existing tags unchanged (frontmatter fallback removed in B2).
func TestUpdateDocument_ExplicitTagsParam(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	ctx := context.Background()
	seed, _ := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u", Type: domain.DocFree, Path: "n", Tags: []string{"old"},
	})
	uc := usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	newTags := []string{"new"}
	got, err := uc.Execute(ctx, "u", seed.ID, usecase.UpdateDocumentInput{
		Title: "N", Body: "body", Tags: &newTags,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"new"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %#v, want %#v", got.Tags, want)
	}
}

// TestUpdateDocument_NilTagsLeavesExistingUnchanged verifies that when in.Tags
// is nil, the persisted taggings in the tag store are not modified.
// NOTE: asserting against FakeTagStore (source of truth), not the returned doc —
// the real pgstore.Update returns Tags=nil (column dropped; Update doesn't
// hydrate), so checking the return value would pass for the wrong reason.
func TestUpdateDocument_NilTagsLeavesExistingUnchanged(t *testing.T) {
	t.Parallel()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	ctx := context.Background()
	seed, _ := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u", Type: domain.DocFree, Path: "unchanged",
	})
	// Seed an existing tag set via the tag store (the source of truth).
	if _, err := tags.SetTags(ctx, "u", domain.TaggableDocument, seed.ID, []string{"kept"}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	uc := usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	if _, err := uc.Execute(ctx, "u", seed.ID, usecase.UpdateDocumentInput{
		Title: "N", Body: "---\ntags: [ignored]\n---\nbody",
		// Tags: nil — must NOT touch taggings
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// Assert against the tag store — not the returned doc.
	got, err := tags.TagsFor(ctx, "u", domain.TaggableDocument, seed.ID)
	if err != nil {
		t.Fatalf("TagsFor: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "kept" {
		t.Fatalf("taggings = %v, want [{slug:kept}] (nil-tags update must not touch taggings)", got)
	}
}

func TestCreateDocument_FrontmatterWikilinkNotExtracted(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.CreateDocument{Docs: store, IDs: ids, Clock: clk}

	// Body has [[ghost]] INSIDE frontmatter (quoted to keep valid YAML) and
	// [[real]] in the actual body text. Only "real" should be indexed.
	got, err := uc.Execute(ctx, "u", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "note", Title: "N",
		Body: "---\ntags: [go]\nref: \"[[ghost]]\"\n---\nsee [[real]] here",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Seed target docs so Backlinks can resolve them.
	_, _ = store.Create(ctx, domain.Document{
		ID: "ghost-doc", OwnerID: "u", Type: domain.DocFree, Path: "ghost",
		Title: "Ghost", CreatedAt: clk.T, UpdatedAt: clk.T,
	})
	_, _ = store.Create(ctx, domain.Document{
		ID: "real-doc", OwnerID: "u", Type: domain.DocFree, Path: "real",
		Title: "Real", CreatedAt: clk.T, UpdatedAt: clk.T,
	})

	bl := usecase.Backlinks{Docs: store}

	// "real" must appear as a backlink of got
	realRefs, err := bl.Execute(ctx, "u", "real-doc")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range realRefs {
		if d.ID == got.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q to be a backlink of 'real', backlinks = %v", got.ID, realRefs)
	}

	// "ghost" must NOT appear as a backlink — it was only inside frontmatter
	ghostRefs, err := bl.Execute(ctx, "u", "ghost-doc")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ghostRefs {
		if d.ID == got.ID {
			t.Errorf("got %q incorrectly recorded as a backlink of 'ghost' (frontmatter wikilink leaked)", got.ID)
		}
	}
}

func TestCreateDocument_StripsHighlightSentinels(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	got, err := uc.Execute(context.Background(), "u", usecase.CreateDocumentInput{
		Type:  domain.DocFree,
		Path:  "note",
		Title: "Title\x02with\x03sentinels",
		Body:  "Body\x02contains\x03sentinels too",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Title, domain.HighlightStart) || strings.Contains(got.Title, domain.HighlightEnd) {
		t.Errorf("Title still contains sentinel: %q", got.Title)
	}
	if strings.Contains(got.Body, domain.HighlightStart) || strings.Contains(got.Body, domain.HighlightEnd) {
		t.Errorf("Body still contains sentinel: %q", got.Body)
	}
	if got.Title != "Titlewithsentinels" {
		t.Errorf("Title = %q, want Titlewithsentinels", got.Title)
	}
	if got.Body != "Bodycontainssentinels too" {
		t.Errorf("Body = %q, want Bodycontainssentinels too", got.Body)
	}
}

func TestUpdateDocument_StripsHighlightSentinels(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	seed, _ := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u", Type: domain.DocFree, Path: "n",
	})
	uc := usecase.UpdateDocument{Docs: docs, Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}}
	got, err := uc.Execute(ctx, "u", seed.ID, usecase.UpdateDocumentInput{
		Title: "Up\x02dated\x03Title",
		Body:  "Up\x02dated\x03Body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Title, domain.HighlightStart) || strings.Contains(got.Title, domain.HighlightEnd) {
		t.Errorf("Title still contains sentinel: %q", got.Title)
	}
	if strings.Contains(got.Body, domain.HighlightStart) || strings.Contains(got.Body, domain.HighlightEnd) {
		t.Errorf("Body still contains sentinel: %q", got.Body)
	}
	if got.Title != "UpdatedTitle" {
		t.Errorf("Title = %q, want UpdatedTitle", got.Title)
	}
	if got.Body != "UpdatedBody" {
		t.Errorf("Body = %q, want UpdatedBody", got.Body)
	}
}

func TestSearchDocuments(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	if _, err := docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Kompendium", Body: "x", Tags: []string{"go"}}); err != nil {
		t.Fatal(err)
	}
	uc := usecase.SearchDocuments{Docs: docs}
	hits, err := uc.Execute(ctx, "u", "kompend", nil, []string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", hits)
	}
}

func TestSearchDocuments_FusesSemanticArm(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Title: "Beta", Body: "totally different"})
	texts := []string{"Beta\n\ntotally different"}
	vecs, _ := emb.Embed(ctx, texts)
	if err := docs.ReplaceChunks(ctx, "b", "u", texts, vecs); err != nil {
		t.Fatal(err)
	}

	uc := usecase.SearchDocuments{Docs: docs, Embedder: emb}
	hits, err := uc.Execute(ctx, "u", "alpha", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, h := range hits {
		got[h.ID] = true
	}
	if !got["a"] || !got["b"] {
		t.Fatalf("fused result should contain a (keyword) and b (semantic): %#v", hits)
	}
}

func TestSearchDocuments_DegradesWhenEmbedderErrors(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.Err = context.DeadlineExceeded
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})

	uc := usecase.SearchDocuments{Docs: docs, Embedder: emb}
	hits, err := uc.Execute(ctx, "u", "alpha", nil, nil)
	if err != nil {
		t.Fatalf("degrade must not error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("degrade should return keyword-only [a], got %#v", hits)
	}
}

func TestSearchDocuments_NilEmbedderIsKeywordOnly(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})
	uc := usecase.SearchDocuments{Docs: docs} // no Embedder
	hits, err := uc.Execute(ctx, "u", "alpha", nil, nil)
	if err != nil || len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("nil embedder → keyword-only [a]; got %#v err %v", hits, err)
	}
}

// countingNotifier records how many times DocumentChanged is called.
type countingNotifier struct{ n int }

func (c *countingNotifier) DocumentChanged() { c.n++ }

func TestCreateDocument_NotifiesOnWrite(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	note := &countingNotifier{}
	ctx := context.Background()
	uc := usecase.CreateDocument{
		Docs:     docs,
		IDs:      &testutil.FakeIDGen{},
		Clock:    testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)},
		Notifier: note,
	}
	if _, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/test", Title: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	if note.n != 1 {
		t.Fatalf("expected 1 notify, got %d", note.n)
	}
}

func TestUpdateDocument_NotifiesOnWrite(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	seed, _ := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u", Type: domain.DocFree, Path: "n", Title: "Old",
		CreatedAt: clk.T, UpdatedAt: clk.T,
	})
	note := &countingNotifier{}
	uc := usecase.UpdateDocument{Docs: docs, Clock: clk, Notifier: note}
	if _, err := uc.Execute(ctx, "u", seed.ID, usecase.UpdateDocumentInput{
		Title: "New", Body: "new body",
	}); err != nil {
		t.Fatal(err)
	}
	if note.n != 1 {
		t.Fatalf("expected 1 notify, got %d", note.n)
	}
}

func TestListDocuments_TagFilter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	for _, d := range []domain.Document{
		{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}},
		{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Tags: []string{"go"}},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.ListDocuments{Docs: docs}
	got, err := uc.Execute(ctx, "u", nil, []string{"go", "tui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", got)
	}
}
