package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateDocument_LinkFailureRollsBackDocument(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "links"
	uc := usecase.CreateDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)},
	}

	_, err := uc.Execute(ctx, "owner", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "notes/atomic", Title: "Atomic", Body: "[[target]]",
	})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want link failure, got %v", err)
	}
	got, listErr := docs.List(ctx, "owner", nil)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(got) != 0 {
		t.Fatalf("document survived failed link write: %+v", got)
	}
}

func TestUpdateDocument_LinkFailureRollsBackDocument(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	seed := domain.Document{
		ID: "doc-1", OwnerID: "owner", Type: domain.DocFree, Path: "notes/atomic",
		Title: "Before", Body: "[[before]]", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := docs.Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if err := docs.ReplaceLinks(ctx, seed.ID, seed.OwnerID, []string{"before"}); err != nil {
		t.Fatal(err)
	}
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "links"
	uc := usecase.UpdateDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		Clock: testutil.FakeClock{T: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)},
	}

	_, err := uc.Execute(ctx, "owner", seed.ID, usecase.UpdateDocumentInput{Title: "After", Body: "[[after]]"})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want link failure, got %v", err)
	}
	got, getErr := docs.Get(ctx, "owner", seed.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.Title != seed.Title || got.Body != seed.Body {
		t.Fatalf("document changed despite failed link write: %+v", got)
	}
	if links := docs.LinksFor(seed.ID); len(links) != 1 || links[0] != "before" {
		t.Fatalf("links changed after rollback: %v", links)
	}
}

func TestPatchDocument_UpdatesOnlySuppliedFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	updatedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	seed := domain.Document{
		ID: "doc-patch", OwnerID: "owner", Type: domain.DocMemory, Path: "memory/patch",
		Title: "Before", Body: "keep this body", CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	if _, err := docs.Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	uc := usecase.UpdateDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		Clock: testutil.FakeClock{T: updatedAt.Add(time.Minute)},
	}
	newTags := []string{"reliable"}

	got, err := uc.ExecutePatch(ctx, "owner", seed.ID, usecase.PatchDocumentInput{
		Tags: &newTags, ExpectedUpdatedAt: &updatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != seed.Title || got.Body != seed.Body {
		t.Fatalf("tags-only patch clobbered content: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "reliable" {
		t.Fatalf("tags = %v, want [reliable]", got.Tags)
	}
}

func TestPatchDocument_RejectsStaleExpectedUpdatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	updatedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	seed := domain.Document{
		ID: "doc-conflict", OwnerID: "owner", Type: domain.DocMemory, Path: "memory/conflict",
		Title: "Current", Body: "current body", CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	if _, err := docs.Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	tags := testutil.NewFakeTagStore()
	uc := usecase.UpdateDocument{
		Docs: docs, Aggregate: testutil.NewFakeDocumentAggregateStore(docs, tags), Tags: tags,
		Clock: testutil.FakeClock{T: updatedAt.Add(time.Minute)},
	}
	stale := updatedAt.Add(-time.Minute)
	title := "Stale overwrite"

	_, err := uc.ExecutePatch(ctx, "owner", seed.ID, usecase.PatchDocumentInput{
		Title: &title, ExpectedUpdatedAt: &stale,
	})
	if !errors.Is(err, ports.ErrDocumentConflict) {
		t.Fatalf("error = %v, want ErrDocumentConflict", err)
	}
	got, getErr := docs.Get(ctx, "owner", seed.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.Title != seed.Title || got.Body != seed.Body {
		t.Fatalf("stale patch changed document: %+v", got)
	}
}

func TestPatchDocument_ConcurrentCASAllowsOneWriter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	updatedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	seed := domain.Document{
		ID: "doc-race", OwnerID: "owner", Type: domain.DocMemory, Path: "memory/race",
		Title: "Current", Body: "current body", CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	if _, err := docs.Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	tags := testutil.NewFakeTagStore()
	uc := usecase.UpdateDocument{
		Docs: docs, Aggregate: testutil.NewFakeDocumentAggregateStore(docs, tags), Tags: tags,
		Clock: testutil.FakeClock{T: updatedAt},
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, title := range []string{"First", "Second"} {
		title := title
		go func() {
			<-start
			_, err := uc.ExecutePatch(ctx, "owner", seed.ID, usecase.PatchDocumentInput{
				Title: &title, ExpectedUpdatedAt: &updatedAt,
			})
			errs <- err
		}()
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			successes++
		case errors.Is(err, ports.ErrDocumentConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent patch error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
}

func TestCreateDocument_TagFailureRollsBackDocumentAndLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "tags"
	uc := usecase.CreateDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()},
	}

	_, err := uc.Execute(ctx, "owner", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "notes/tag-rollback", Body: "[[target]]", Tags: []string{"atomic"},
	})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want tag failure, got %v", err)
	}
	got, listErr := docs.List(ctx, "owner", nil)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(got) != 0 {
		t.Fatalf("document survived failed tag write: %+v", got)
	}
}

func TestCreateDocument_CommitFailureRollsBackAndDoesNotNotify(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "commit"
	note := &countingNotifier{}
	uc := usecase.CreateDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}, Notifier: note,
	}

	_, err := uc.Execute(ctx, "owner", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "notes/commit", Body: "[[target]]", Tags: []string{"atomic"},
	})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want commit failure, got %v", err)
	}
	if got, _ := docs.List(ctx, "owner", nil); len(got) != 0 {
		t.Fatalf("document survived failed commit: %+v", got)
	}
	if note.n != 0 {
		t.Fatalf("notifier called before commit: %d", note.n)
	}
}

func TestUpdateDocument_AllFollowFailuresRollBack(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"document", "links", "tags", "commit"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			docs := testutil.NewFakeDocumentStore()
			tags := testutil.NewFakeTagStore()
			seed := domain.Document{
				ID: "doc-1", OwnerID: "owner", Type: domain.DocFree, Path: "notes/update",
				Title: "Before", Body: "[[before]]", Tags: []string{"old"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}
			if _, err := docs.Create(ctx, seed); err != nil {
				t.Fatal(err)
			}
			if err := docs.ReplaceLinks(ctx, seed.ID, seed.OwnerID, []string{"before"}); err != nil {
				t.Fatal(err)
			}
			if _, err := tags.SetTags(ctx, seed.OwnerID, domain.TaggableDocument, seed.ID, []string{"old"}); err != nil {
				t.Fatal(err)
			}
			aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
			aggregate.FailStage = stage
			note := &countingNotifier{}
			newTags := []string{"new"}
			uc := usecase.UpdateDocument{
				Docs: docs, Aggregate: aggregate, Tags: tags,
				Clock: testutil.FakeClock{T: seed.UpdatedAt.Add(time.Hour)}, Notifier: note,
			}

			_, err := uc.Execute(ctx, seed.OwnerID, seed.ID, usecase.UpdateDocumentInput{
				Title: "After", Body: "[[after]]", Tags: &newTags,
			})
			if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
				t.Fatalf("want %s failure, got %v", stage, err)
			}
			got, err := docs.Get(ctx, seed.OwnerID, seed.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != seed.Title || got.Body != seed.Body || !got.UpdatedAt.Equal(seed.UpdatedAt) {
				t.Fatalf("document changed after %s rollback: %+v", stage, got)
			}
			if links := docs.LinksFor(seed.ID); len(links) != 1 || links[0] != "before" {
				t.Fatalf("links changed after %s rollback: %v", stage, links)
			}
			gotTags, err := tags.TagsFor(ctx, seed.OwnerID, domain.TaggableDocument, seed.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(gotTags) != 1 || gotTags[0].Slug != "old" {
				t.Fatalf("tags changed after %s rollback: %+v", stage, gotTags)
			}
			if note.n != 0 {
				t.Fatalf("notifier called after %s failure: %d", stage, note.n)
			}
		})
	}
}

func TestImportDocument_CommitFailureRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "commit"
	uc := usecase.ImportDocument{
		Docs: docs, Aggregate: aggregate, Tags: tags,
		IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()},
	}

	_, err := uc.Execute(ctx, "owner", usecase.ImportDocumentInput{
		Type: domain.DocFree, Path: "imports/atomic", Body: "[[target]]", Tags: []string{"imported"},
	})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want commit failure, got %v", err)
	}
	if got, _ := docs.List(ctx, "owner", nil); len(got) != 0 {
		t.Fatalf("import survived failed commit: %+v", got)
	}
}

func TestUpsertDocumentByPath_AllFollowFailuresRollBack(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"document", "links", "tags", "curation", "commit"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			docs := testutil.NewFakeDocumentStore()
			tags := testutil.NewFakeTagStore()
			id, _, err := docs.UpsertByPath(ctx, "owner", nil, domain.DocMemory, "memory/atomic", "Before", "[[before]]", false, false, "human", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := docs.ReplaceLinks(ctx, id, "owner", []string{"before"}); err != nil {
				t.Fatal(err)
			}
			if _, err := tags.SetTags(ctx, "owner", domain.TaggableDocument, id, []string{"old"}); err != nil {
				t.Fatal(err)
			}
			aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
			aggregate.FailStage = stage
			note := &countingNotifier{}
			uc := usecase.UpsertDocumentByPath{Docs: docs, Aggregate: aggregate, Tags: tags, Notifier: note}

			_, _, err = uc.Execute(ctx, "owner", usecase.UpsertByPathInput{
				Type: domain.DocMemory, Path: "memory/atomic", Title: "After", Body: "[[after]]",
				Tags: []string{"new"}, Pinned: true,
			})
			if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
				t.Fatalf("want %s failure, got %v", stage, err)
			}
			got, err := docs.Get(ctx, "owner", id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != "Before" || got.Body != "[[before]]" || got.Pinned || got.Archived {
				t.Fatalf("upsert changed document after %s rollback: %+v", stage, got)
			}
			if links := docs.LinksFor(id); len(links) != 1 || links[0] != "before" {
				t.Fatalf("upsert changed links after %s rollback: %v", stage, links)
			}
			gotTags, err := tags.TagsFor(ctx, "owner", domain.TaggableDocument, id)
			if err != nil {
				t.Fatal(err)
			}
			if len(gotTags) != 1 || gotTags[0].Slug != "old" {
				t.Fatalf("upsert changed tags after %s rollback: %+v", stage, gotTags)
			}
			if note.n != 0 {
				t.Fatalf("notifier called after %s failure: %d", stage, note.n)
			}
		})
	}
}

func TestSetActiveContext_TagFailureRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(ctx, domain.Node{ID: "repo-1", OwnerID: "owner", Kind: domain.KindRepo, Name: "Flow", Slug: "flow"}); err != nil {
		t.Fatal(err)
	}
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "tags"
	uc := usecase.SetActiveContext{Nodes: nodes, Docs: docs, Aggregate: aggregate, Tags: tags}

	_, _, err := uc.Execute(ctx, "owner", usecase.ContextResolveInput{NodeOverride: "flow"}, "", "[[next]]", []string{"handoff"})
	if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
		t.Fatalf("want tag failure, got %v", err)
	}
	if got, _ := docs.List(ctx, "owner", nil); len(got) != 0 {
		t.Fatalf("active context survived failed tag write: %+v", got)
	}
}

func TestDeleteDocument_AllFailuresRollBack(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"tags", "document", "commit"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			docs := testutil.NewFakeDocumentStore()
			tags := testutil.NewFakeTagStore()
			seed := domain.Document{ID: "doc-1", OwnerID: "owner", Type: domain.DocFree, Path: "delete/atomic"}
			if _, err := docs.Create(ctx, seed); err != nil {
				t.Fatal(err)
			}
			if _, err := tags.SetTags(ctx, seed.OwnerID, domain.TaggableDocument, seed.ID, []string{"keep"}); err != nil {
				t.Fatal(err)
			}
			aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
			aggregate.FailStage = stage
			uc := usecase.DeleteDocument{Docs: docs, Aggregate: aggregate, Tags: tags}

			err := uc.Execute(ctx, seed.OwnerID, seed.ID)
			if !errors.Is(err, testutil.ErrFakeDocumentAggregate) {
				t.Fatalf("want %s failure, got %v", stage, err)
			}
			if _, err := docs.Get(ctx, seed.OwnerID, seed.ID); err != nil {
				t.Fatalf("document lost after %s rollback: %v", stage, err)
			}
			gotTags, err := tags.TagsFor(ctx, seed.OwnerID, domain.TaggableDocument, seed.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(gotTags) != 1 || gotTags[0].Slug != "keep" {
				t.Fatalf("tags lost after %s rollback: %+v", stage, gotTags)
			}
		})
	}
}
