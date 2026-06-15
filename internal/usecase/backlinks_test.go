package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBacklinks_FiltersByScope(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Now()
	pA := "proj-a"

	mk := func(id, path string, proj *string) {
		if _, err := store.Create(ctx, domain.Document{
			ID: id, OwnerID: "o", ProjectID: proj, Type: domain.DocFree,
			Path: path, Title: id, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("target", "spec", nil)
	mk("good", "g", nil)
	mk("bad", "b", &pA)
	mk("decoy", "d", &pA)

	_ = store.ReplaceLinks(ctx, "good", "o", []string{"spec"})
	_ = store.ReplaceLinks(ctx, "bad", "o", []string{"spec"})
	_ = store.ReplaceLinks(ctx, "decoy", "o", []string{"other"})

	uc := usecase.Backlinks{Docs: store}
	refs, err := uc.Execute(ctx, "o", "target")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids["good"] || !ids["bad"] || ids["decoy"] || len(refs) != 2 {
		t.Fatalf("backlinks = %v, want good+bad", ids)
	}
}

func TestBacklinks_DropsForeignProjectFalsePositive(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Now()
	pA, pB := "proj-a", "proj-b"
	mk := func(id, path string, proj *string) {
		_, _ = store.Create(ctx, domain.Document{
			ID: id, OwnerID: "o", ProjectID: proj, Type: domain.DocFree,
			Path: path, Title: id, CreatedAt: now, UpdatedAt: now,
		})
	}
	mk("notesA", "notes", &pA)
	mk("notesB", "notes", &pB)
	mk("srcB", "s", &pB)
	_ = store.ReplaceLinks(ctx, "srcB", "o", []string{"notes"})

	uc := usecase.Backlinks{Docs: store}
	refs, err := uc.Execute(ctx, "o", "notesA")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("notesA should have no backlinks (srcB resolves to notesB), got %v", refs)
	}
}
