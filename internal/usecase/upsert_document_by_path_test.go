package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestUpsertDocumentByPath_RejectsForeignNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(ctx, domain.Node{ID: "n2", OwnerID: "u2", Kind: domain.KindEngagement, Name: "Foreign", Slug: "foreign", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n2"
	uc := usecase.UpsertDocumentByPath{Docs: docs, Tags: testutil.NewFakeTagStore(), Nodes: nodes}
	_, _, err := uc.Execute(ctx, "u1", usecase.UpsertByPathInput{Type: domain.DocProject, NodeID: &nodeID, Path: "projects/foreign", Title: "T", Body: "B"})
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}

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

func TestUpsertDocumentByPath_StampsActor(t *testing.T) {
	store := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.UpsertDocumentByPath{Docs: store, Tags: tags}

	ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Agent, Ref: "claude-code"})
	id, _, err := uc.Execute(ctx, "owner", usecase.UpsertByPathInput{
		Type: domain.DocMemory, Path: "prov-upsert", Title: "T", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := store.Get(context.Background(), "owner", id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.UpdatedByKind != "agent" || doc.UpdatedByRef != "claude-code" {
		t.Fatalf("UpdatedByKind/Ref = %q/%q, want agent/claude-code", doc.UpdatedByKind, doc.UpdatedByRef)
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
