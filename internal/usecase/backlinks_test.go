package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// partialErrDocStore wraps FakeDocumentStore and can inject errors for specific methods.
type partialErrDocStore struct {
	*testutil.FakeDocumentStore
	backlinksErr error
	listErr      error
}

func (s *partialErrDocStore) Backlinks(ctx context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	if s.backlinksErr != nil {
		return nil, s.backlinksErr
	}
	return s.FakeDocumentStore.Backlinks(ctx, ownerID, targetPath)
}

func (s *partialErrDocStore) List(ctx context.Context, ownerID string, nodeID *string, tags ...string) ([]domain.Document, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.FakeDocumentStore.List(ctx, ownerID, nodeID, tags...)
}

func TestBacklinks_FiltersByScope(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Now()
	pA := "proj-a"

	mk := func(id, path string, proj *string) {
		if _, err := store.Create(ctx, domain.Document{
			ID: id, OwnerID: "o", NodeID: proj, Type: domain.DocFree,
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
			ID: id, OwnerID: "o", NodeID: proj, Type: domain.DocFree,
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

func TestBacklinks_GetNotFound(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	uc := usecase.Backlinks{Docs: store}
	// Target doc does not exist: Get returns ErrDocumentNotFound
	_, err := uc.Execute(ctx, "o", "no-such-id")
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("want ErrDocumentNotFound, got %v", err)
	}
}

func TestBacklinks_BacklinksStoreError(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	base := testutil.NewFakeDocumentStore()
	_, _ = base.Create(ctx, domain.Document{
		ID: "t", OwnerID: "o", Type: domain.DocFree,
		Path: "target", Title: "T", CreatedAt: now, UpdatedAt: now,
	})
	store := &partialErrDocStore{FakeDocumentStore: base, backlinksErr: errors.New("backlinks fail")}
	uc := usecase.Backlinks{Docs: store}
	_, err := uc.Execute(ctx, "o", "t")
	if err == nil {
		t.Fatal("want error from Backlinks, got nil")
	}
}

func TestBacklinks_ListStoreError(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	base := testutil.NewFakeDocumentStore()
	_, _ = base.Create(ctx, domain.Document{
		ID: "t", OwnerID: "o", Type: domain.DocFree,
		Path: "target", Title: "T", CreatedAt: now, UpdatedAt: now,
	})
	store := &partialErrDocStore{FakeDocumentStore: base, listErr: errors.New("list fail")}
	uc := usecase.Backlinks{Docs: store}
	_, err := uc.Execute(ctx, "o", "t")
	if err == nil {
		t.Fatal("want error from List, got nil")
	}
}
