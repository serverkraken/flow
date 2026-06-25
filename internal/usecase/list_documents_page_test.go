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
