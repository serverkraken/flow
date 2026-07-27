package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBulkCurateDocumentsValidatesBeforeAtomicStoreMutation(t *testing.T) {
	ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Agent, Ref: "codex"})
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	docs := testutil.NewFakeDocumentStore()
	for _, doc := range []domain.Document{
		{ID: "ours-1", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/one", Title: "One", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "ours-2", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/two", Title: "Two", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "foreign", OwnerID: "u2", Type: domain.DocMemory, Path: "memory/foreign", Title: "Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.BulkCurateDocuments{Docs: docs, Clock: testutil.FakeClock{T: now.Add(time.Hour)}}
	archived := true

	_, err := uc.Execute(ctx, "u1", usecase.BulkCurateDocumentsInput{IDs: []string{"ours-1", "foreign"}, Archived: &archived})
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("mixed-owner bulk error = %v, want ErrDocumentNotFound", err)
	}
	ours, err := docs.Get(ctx, "u1", "ours-1")
	if err != nil || ours.Archived || !ours.Pinned {
		t.Fatalf("failed bulk mutation was not rolled back: %+v err=%v", ours, err)
	}

	changed, err := uc.Execute(ctx, "u1", usecase.BulkCurateDocumentsInput{IDs: []string{"ours-2", "ours-1", "ours-1"}, Archived: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 || changed[0].ID != "ours-2" || changed[1].ID != "ours-1" {
		t.Fatalf("bulk result order/dedup = %+v", changed)
	}
	for _, id := range []string{"ours-1", "ours-2"} {
		doc, err := docs.Get(ctx, "u1", id)
		if err != nil {
			t.Fatal(err)
		}
		if !doc.Archived || doc.Pinned || doc.ArchivedAt == nil || !doc.ArchivedAt.Equal(now.Add(time.Hour)) || doc.UpdatedByKind != "agent" || doc.UpdatedByRef != "codex" {
			t.Fatalf("archived document %s lacks atomic state/provenance: %+v", id, doc)
		}
	}
}

func TestBulkCurateDocumentsRejectsInvalidAndNonContextDocuments(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "free", OwnerID: "u1", Type: domain.DocFree, Path: "free/one", Title: "Free", CreatedAt: now, UpdatedAt: now})
	uc := usecase.BulkCurateDocuments{Docs: docs, Clock: testutil.FakeClock{T: now}}
	mode := domain.ContextModeImmer
	tooMany := make([]string, usecase.MaxBulkDocumentCuration+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("doc-%d", i)
	}

	for name, in := range map[string]usecase.BulkCurateDocumentsInput{
		"empty":        {Archived: boolPtr(true)},
		"over limit":   {IDs: tooMany, Archived: boolPtr(true)},
		"two actions":  {IDs: []string{"free"}, Archived: boolPtr(true), ContextMode: &mode},
		"invalid mode": {IDs: []string{"free"}, ContextMode: contextModePtr("invalid")},
		"wrong type":   {IDs: []string{"free"}, ContextMode: &mode},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := uc.Execute(ctx, "u1", in); !errors.Is(err, domain.ErrInvalidDocument) {
				t.Fatalf("Execute error = %v, want ErrInvalidDocument", err)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func contextModePtr(v domain.ContextMode) *domain.ContextMode { return &v }
