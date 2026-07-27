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

func TestSetContextMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p"})
	uc := usecase.SetContextMode{Docs: docs}

	if err := uc.Execute(ctx, "u1", "d1", domain.ContextModeImmer); err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", "d1")
	if got.ContextMode != domain.ContextModeImmer {
		t.Fatalf("context mode not set, got %q", got.ContextMode)
	}
}

func TestSetContextMode_InvalidMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p"})
	uc := usecase.SetContextMode{Docs: docs}

	if err := uc.Execute(ctx, "u1", "d1", domain.ContextMode("bogus")); !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("want ErrInvalidDocument, got %v", err)
	}
	got, _ := docs.Get(ctx, "u1", "d1")
	if got.ContextMode == domain.ContextMode("bogus") {
		t.Fatalf("store must not be called with invalid mode")
	}
}

func TestSetContextMode_OwnerScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p"})
	uc := usecase.SetContextMode{Docs: docs}

	if err := uc.Execute(ctx, "u2", "d1", domain.ContextModeImmer); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("want ErrDocumentNotFound, got %v", err)
	}
}
