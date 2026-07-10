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

func TestGetArtifact_Roundtrip(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "diagram", Name: "Diagram.png",
		Mime: "image/png", Bytes: []byte("bytes"),
	}); err != nil {
		t.Fatal(err)
	}
	uc := usecase.GetArtifact{Artifacts: as}

	got, err := uc.Execute(ctx, "u1", "n1", "diagram")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes) != "bytes" {
		t.Errorf("bytes = %q, want bytes (Get includes payload for the serve route)", got.Bytes)
	}
}

func TestGetArtifact_NotFound(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	uc := usecase.GetArtifact{Artifacts: as}

	_, err := uc.Execute(ctx, "u1", "n1", "ghost")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound", err)
	}
}

func TestGetArtifact_ForeignOwner_NotFound(t *testing.T) {
	ctx := context.Background()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "diagram", Name: "Diagram.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	uc := usecase.GetArtifact{Artifacts: as}

	_, err := uc.Execute(ctx, "u1", "n1", "diagram")
	if !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("err = %v, want ErrArtifactNotFound (cross-tenant)", err)
	}
}
