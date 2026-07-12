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

func TestDeleteArtifact_RoundtripEmitsDeleted(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "diagram", Name: "Diagram.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.DeleteArtifact{Artifacts: as, Emitter: em}

	if err := uc.Execute(ctx, "u1", "n1", "diagram"); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Get(ctx, "u1", "n1", "diagram"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("artifact still present after delete: %v", err)
	}
	if len(em.events) != 1 || em.events[0].Type != domain.EventArtifactDeleted {
		t.Fatalf("want exactly one artifact.deleted, got %+v", em.events)
	}
	if em.events[0].Data["node"] != "n1" || em.events[0].Data["id"] != "diagram" {
		t.Errorf("event data = %+v, want node=n1 id=diagram", em.events[0].Data)
	}
}

func TestDeleteArtifact_NotFound_NoEvent(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.DeleteArtifact{Artifacts: as, Emitter: em}

	err := uc.Execute(ctx, "u1", "n1", "ghost")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound", err)
	}
	if len(em.events) != 0 {
		t.Errorf("want no event on missing artifact, got %+v", em.events)
	}
}

func TestDeleteArtifact_ForeignOwner_NotFound(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "diagram", Name: "Diagram.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.DeleteArtifact{Artifacts: as, Emitter: em}

	err := uc.Execute(ctx, "u1", "n1", "diagram")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound (cross-tenant)", err)
	}
	if _, gerr := as.Get(ctx, "other-owner", "n1", "diagram"); gerr != nil {
		t.Error("foreign owner's artifact must NOT have been deleted")
	}
}
