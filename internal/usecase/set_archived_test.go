package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetArchived(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p", Title: "T"})
	uc := usecase.SetArchived{Docs: docs}
	if err := uc.Execute(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", "d1")
	if !got.Archived {
		t.Fatalf("not archived: %+v", got)
	}
}
