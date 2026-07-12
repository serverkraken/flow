package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestRenameArtifact_UpdatesNameKeepsSlugAndRef(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "diagram", Name: "Diagram.png",
		Mime: "image/png", Ref: "abc123def456",
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.RenameArtifact{Artifacts: as, Emitter: em}

	if err := uc.Execute(ctx, "u1", "n1", "diagram", "Renamed Diagram"); err != nil {
		t.Fatal(err)
	}
	got, err := as.GetMeta(ctx, "u1", "n1", "diagram")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed Diagram" {
		t.Errorf("name = %q, want Renamed Diagram", got.Name)
	}
	if got.Slug != "diagram" || got.Ref != "abc123def456" {
		t.Errorf("slug/ref changed: got slug=%q ref=%q", got.Slug, got.Ref)
	}
	if len(em.events) != 1 || em.events[0].Type != domain.EventArtifactUpdated {
		t.Fatalf("want exactly one artifact.updated, got %+v", em.events)
	}
}

func TestRenameArtifact_ForeignNode_NotFound(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	// Artifact hangs off "n-ancestor", not "n1" — GetMeta at n1 must fail even
	// though a document at n1 could see it via the ancestor chain (ListArtifacts).
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n-ancestor", Slug: "diagram", Name: "Diagram.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.RenameArtifact{Artifacts: as, Emitter: em}

	err := uc.Execute(ctx, "u1", "n1", "diagram", "Renamed")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound", err)
	}
	if len(em.events) != 0 {
		t.Errorf("want no event, got %+v", em.events)
	}
}

func TestRenameArtifact_ForeignOwner_NotFound(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "diagram", Name: "Diagram.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.RenameArtifact{Artifacts: as, Emitter: em}

	err := uc.Execute(ctx, "u1", "n1", "diagram", "Renamed")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound", err)
	}
}
