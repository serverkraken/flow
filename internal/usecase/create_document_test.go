package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateDocument_RejectsForeignNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(ctx, domain.Node{ID: "n2", OwnerID: "u2", Kind: domain.KindEngagement, Name: "Foreign", Slug: "foreign", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n2"
	uc := usecase.CreateDocument{Docs: docs, Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}
	_, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{Type: domain.DocProject, NodeID: &nodeID, Path: "projects/foreign", Title: "T", Body: "B"})
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
	if got, _ := docs.List(ctx, "u1", nil); len(got) != 0 {
		t.Fatalf("foreign project document was persisted: %+v", got)
	}
}

func TestCreateDocument_ExplicitTagsParam(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.CreateDocument{Docs: docs, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}
	got, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "p", Title: "T", Body: "pure content, no frontmatter",
		Tags: []string{"Go", "tui"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go", "tui"}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("response tags = %v, want %v", got.Tags, want)
	}
	stored, _ := tags.TagsFor(ctx, "u1", domain.TaggableDocument, got.ID)
	if len(stored) != 2 {
		t.Fatalf("taggings want 2, got %+v", stored)
	}
}

func TestCreateDocument_StampsActor(t *testing.T) {
	t.Parallel()
	ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Agent, Ref: "claude-code"})
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	got, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "p2", Title: "T", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedByKind != "agent" || got.UpdatedByRef != "claude-code" {
		t.Fatalf("UpdatedByKind/Ref = %q/%q, want agent/claude-code", got.UpdatedByKind, got.UpdatedByRef)
	}
}

func TestCreateDocument_StampsActor_DefaultsHumanWhenNoActorInContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background() // no actor.WithContext -> FromContext default {Kind: Human}
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	got, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "p3", Title: "T", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedByKind != "human" {
		t.Fatalf("UpdatedByKind = %q, want human (default, no crash)", got.UpdatedByKind)
	}
}
