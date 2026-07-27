package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestMoveDocument_ReclassifiesAtomically(t *testing.T) {
	ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Human, Ref: "Soenne"})
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if _, err := nodes.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatal(err)
	}
	if _, err := docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/old"}); err != nil {
		t.Fatal(err)
	}

	uc := usecase.MoveDocument{Docs: docs, Nodes: nodes, Clock: testutil.FakeClock{T: now}}
	got, err := uc.Execute(ctx, "u1", "d1", usecase.MoveDocumentInput{
		Type: domain.DocProject, NodeID: stringPtr("n1"), Path: "readme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != domain.DocProject || got.NodeID == nil || *got.NodeID != "n1" || got.Path != "readme" || got.Date != nil {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if !got.UpdatedAt.Equal(now) || got.UpdatedByKind != "human" || got.UpdatedByRef != "Soenne" {
		t.Fatalf("move provenance/timestamp not stamped: %+v", got)
	}
}

func TestMoveDocument_RejectsForeignNodeWithoutChangingDocument(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(ctx, domain.Node{ID: "foreign", OwnerID: "u2", Name: "Foreign", Slug: "foreign", Kind: domain.KindRepo}); err != nil {
		t.Fatal(err)
	}
	original := domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/original"}
	if _, err := docs.Create(ctx, original); err != nil {
		t.Fatal(err)
	}

	uc := usecase.MoveDocument{Docs: docs, Nodes: nodes, Clock: testutil.FakeClock{T: time.Now()}}
	_, err := uc.Execute(ctx, "u1", "d1", usecase.MoveDocumentInput{
		Type: domain.DocProject, NodeID: stringPtr("foreign"), Path: "readme",
	})
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
	got, getErr := docs.Get(ctx, "u1", "d1")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.Type != original.Type || got.Path != original.Path || got.NodeID != nil {
		t.Fatalf("document changed after rejected move: %+v", got)
	}
}

func TestMoveDocument_DetectsDestinationCollision(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	for _, d := range []domain.Document{
		{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/source"},
		{ID: "d2", OwnerID: "u1", Type: domain.DocFree, Path: "notes/taken"},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.MoveDocument{Docs: docs, Clock: testutil.FakeClock{T: time.Now()}}
	_, err := uc.Execute(ctx, "u1", "d1", usecase.MoveDocumentInput{Type: domain.DocFree, Path: "notes/taken"})
	if !errors.Is(err, ports.ErrDocumentExists) {
		t.Fatalf("want ErrDocumentExists, got %v", err)
	}
}

func stringPtr(v string) *string { return &v }
