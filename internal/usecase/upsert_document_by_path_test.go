package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestUpsertDocumentByPath_Idempotent(t *testing.T) {
	store := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.UpsertDocumentByPath{Docs: store, Tags: tags}

	in := usecase.UpsertByPathInput{
		Type: domain.DocMemory, NodeID: nil, Path: "feedback_no_icons",
		Title: "No emoji", Body: "avoid colored emoji [[feedback_no_monoliths]]",
		Tags: []string{"feedback"}, Pinned: true,
	}
	id1, _, err := uc.Execute(context.Background(), "owner", in)
	if err != nil {
		t.Fatal(err)
	}
	// re-run with same path -> same id (upsert, not duplicate)
	id2, _, err := uc.Execute(context.Background(), "owner", in)
	if err != nil || id1 != id2 {
		t.Fatalf("not idempotent: id1=%s id2=%s err=%v", id1, id2, err)
	}
	doc, err := store.Get(context.Background(), "owner", id1)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Pinned {
		t.Errorf("pinned not applied")
	}
	if got := store.LinksFor(id1); len(got) != 1 || got[0] != "feedback_no_monoliths" {
		t.Errorf("wikilinks = %v, want [feedback_no_monoliths]", got)
	}
	tagList, err := tags.TagsFor(context.Background(), "owner", domain.TaggableDocument, id1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagList) != 1 || tagList[0].Slug != "feedback" {
		t.Errorf("tags = %v, want [{slug:feedback}]", tagList)
	}
}

func TestUpsertDocumentByPath_Archived(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.UpsertDocumentByPath{Docs: docs, Tags: tags}
	id, _, err := uc.Execute(context.Background(), "u1", usecase.UpsertByPathInput{
		Type: domain.DocMemory, Path: "m1", Title: "M1", Body: "b", Archived: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(context.Background(), "u1", id)
	if !got.Archived {
		t.Fatalf("upsert did not set archived: %+v", got)
	}
	// re-run un-archived → reclassifies
	if _, _, err := uc.Execute(context.Background(), "u1", usecase.UpsertByPathInput{
		Type: domain.DocMemory, Path: "m1", Title: "M1", Body: "b", Archived: false,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = docs.Get(context.Background(), "u1", id)
	if got.Archived {
		t.Fatalf("re-run did not reclassify: %+v", got)
	}
}

func TestUpsertDocumentByPath_RejectsBadType(t *testing.T) {
	uc := usecase.UpsertDocumentByPath{Docs: testutil.NewFakeDocumentStore(), Tags: testutil.NewFakeTagStore()}
	_, _, err := uc.Execute(context.Background(), "owner",
		usecase.UpsertByPathInput{Type: domain.DocumentType("bogus"), Path: "x", Body: "y"})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("err = %v, want ErrInvalidDocument", err)
	}
}
