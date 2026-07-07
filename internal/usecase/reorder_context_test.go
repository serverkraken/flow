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

func TestReorderContextDocs_StampsDensePriorities(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	a, err := docs.Create(ctx, domain.Document{ID: "doc-a", OwnerID: "owner-1", Title: "a", Path: "a.md"})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := docs.Create(ctx, domain.Document{ID: "doc-b", OwnerID: "owner-1", Title: "b", Path: "b.md"})
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	c, err := docs.Create(ctx, domain.Document{ID: "doc-c", OwnerID: "owner-1", Title: "c", Path: "c.md"})
	if err != nil {
		t.Fatalf("create c: %v", err)
	}

	uc := usecase.ReorderContextDocs{Docs: docs}
	if err := uc.Execute(ctx, "owner-1", []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, _ := docs.Get(ctx, "owner-1", c.ID)
	if got.Priority != 3 {
		t.Errorf("c.Priority = %d, want 3", got.Priority)
	}
	got, _ = docs.Get(ctx, "owner-1", a.ID)
	if got.Priority != 2 {
		t.Errorf("a.Priority = %d, want 2", got.Priority)
	}
	got, _ = docs.Get(ctx, "owner-1", b.ID)
	if got.Priority != 1 {
		t.Errorf("b.Priority = %d, want 1", got.Priority)
	}
}

func TestReorderContextDocs_ForeignDocPropagatesError(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	a, _ := docs.Create(ctx, domain.Document{ID: "doc-a", OwnerID: "owner-1", Title: "a", Path: "a.md"})
	foreign, _ := docs.Create(ctx, domain.Document{ID: "doc-x", OwnerID: "owner-2", Title: "foreign", Path: "x.md"})

	uc := usecase.ReorderContextDocs{Docs: docs}
	err := uc.Execute(ctx, "owner-1", []string{a.ID, foreign.ID})
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("Execute: got %v, want ErrDocumentNotFound", err)
	}
}
