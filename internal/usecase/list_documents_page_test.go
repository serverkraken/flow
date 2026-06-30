package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestListDocumentsPage(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	for _, doc := range []domain.Document{
		{ID: "a", OwnerID: "owner", Type: domain.DocFree, Path: "a", Title: "A"},
		{ID: "b", OwnerID: "owner", Type: domain.DocFree, Path: "b", Title: "B"},
		{ID: "c", OwnerID: "owner", Type: domain.DocFree, Path: "c", Title: "C"},
		{ID: "foreign", OwnerID: "other", Type: domain.DocFree, Path: "x", Title: "X"},
	} {
		if _, err := store.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	uc := usecase.NewListDocumentsPage(store)
	docs, total, err := uc.Execute(ctx, "owner", nil, nil, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total=%d want 3", total)
	}
	if len(docs) != 2 {
		t.Fatalf("len=%d want 2", len(docs))
	}
}

func TestListDocumentsPage_DefaultLimitAndOffset(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	for i, id := range []string{"x1", "x2", "x3"} {
		_ = i
		_, _ = store.Create(ctx, domain.Document{
			ID: id, OwnerID: "owner2",
			Type: domain.DocFree, Path: id, Title: id,
		})
	}
	uc := usecase.NewListDocumentsPage(store)
	// limit=0 → should default to 50
	docs, _, err := uc.Execute(ctx, "owner2", nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("default limit: got %d docs, want 3", len(docs))
	}
	// negative offset → should clamp to 0
	docs2, _, err := uc.Execute(ctx, "owner2", nil, nil, 10, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs2) != 3 {
		t.Fatalf("negative offset: got %d docs, want 3", len(docs2))
	}
}
