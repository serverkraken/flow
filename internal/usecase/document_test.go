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

func TestCreateDocument_FreeAndDaily(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	uc := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}

	// free doc
	free, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/architecture", Title: "Arch",
	})
	if err != nil {
		t.Fatalf("free create: %v", err)
	}
	if free.ID == "" {
		t.Error("ID must be stamped")
	}
	if free.OwnerID != "u1" {
		t.Errorf("OwnerID = %q, want u1", free.OwnerID)
	}
	if free.CreatedAt.IsZero() || free.UpdatedAt.IsZero() {
		t.Error("timestamps must be stamped")
	}

	// daily doc — path derived, Date set
	daily, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{Type: domain.DocDaily})
	if err != nil {
		t.Fatalf("daily create: %v", err)
	}
	if daily.Path != "daily/2026-06-15" {
		t.Errorf("daily Path = %q, want daily/2026-06-15", daily.Path)
	}
	if daily.Date == nil {
		t.Error("daily Date must be set")
	}

	// project doc without projectID — must fail Validate
	_, err = uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocProject, Path: "docs/proj",
	})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Errorf("project without projectID: want ErrInvalidDocument, got %v", err)
	}
}

func TestCreateDocument_Persisted(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	get := usecase.GetDocument{Docs: docs}

	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/notes", Title: "Notes",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := get.Execute(ctx, "u1", created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID || got.Title != "Notes" {
		t.Errorf("get returned unexpected doc: %+v", got)
	}
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	t0 := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	clk := testutil.FakeClock{T: t0}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/design", Title: "Old title",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	clk.T = t1
	update := usecase.UpdateDocument{Docs: docs, Clock: clk}
	updated, err := update.Execute(ctx, "u1", created.ID, usecase.UpdateDocumentInput{
		Title: "New title", Body: "new body", Tags: []string{"go"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "New title" {
		t.Errorf("Title = %q, want New title", updated.Title)
	}
	if updated.Body != "new body" {
		t.Errorf("Body = %q, want new body", updated.Body)
	}
	if !updated.UpdatedAt.Equal(t1) {
		t.Errorf("UpdatedAt = %v, want %v", updated.UpdatedAt, t1)
	}
}

func TestUpdateDocument_NotFound(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.UpdateDocument{Docs: docs, Clock: clk}

	_, err := uc.Execute(ctx, "u1", "nonexistent", usecase.UpdateDocumentInput{Title: "X"})
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("want ErrDocumentNotFound, got %v", err)
	}
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	del := usecase.DeleteDocument{Docs: docs}
	get := usecase.GetDocument{Docs: docs}

	created, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/temp", Title: "Temp",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := del.Execute(ctx, "u1", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = get.Execute(ctx, "u1", created.ID)
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("after delete: want ErrDocumentNotFound, got %v", err)
	}
}

func TestCreateDocument_WritesLinks(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.CreateDocument{
		Docs:  store,
		IDs:   ids,
		Clock: clk,
	}
	created, err := uc.Execute(ctx, "o", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "src", Title: "Src", Body: "see [[dest]] and [[dest]]",
	})
	if err != nil {
		t.Fatal(err)
	}
	// FakeIDGen returns "id-1" for the first call
	if created.ID != "id-1" {
		t.Fatalf("expected created.ID = id-1, got %q", created.ID)
	}
	_, _ = store.Create(ctx, domain.Document{
		ID: "doc-2", OwnerID: "o", Type: domain.DocFree, Path: "dest", Title: "Dest",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	refs, err := (usecase.Backlinks{Docs: store}).Execute(ctx, "o", "doc-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "id-1" {
		t.Fatalf("expected src (id-1) as the only backlink of dest, got %v", refs)
	}
}

func TestListDocuments_OwnerScoped(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}

	create := usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk}
	list := usecase.ListDocuments{Docs: docs}

	// u1 creates two docs; u2 creates one
	if _, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/a", Title: "A",
	}); err != nil {
		t.Fatalf("create u1/a: %v", err)
	}
	if _, err := create.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/b", Title: "B",
	}); err != nil {
		t.Fatalf("create u1/b: %v", err)
	}
	if _, err := create.Execute(ctx, "u2", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "docs/c", Title: "C",
	}); err != nil {
		t.Fatalf("create u2/c: %v", err)
	}

	u1Docs, err := list.Execute(ctx, "u1", nil)
	if err != nil {
		t.Fatalf("list u1: %v", err)
	}
	if len(u1Docs) != 2 {
		t.Errorf("u1 list: got %d docs, want 2", len(u1Docs))
	}
	for _, d := range u1Docs {
		if d.OwnerID != "u1" {
			t.Errorf("list returned doc with OwnerID=%q, want u1", d.OwnerID)
		}
	}

	u2Docs, err := list.Execute(ctx, "u2", nil)
	if err != nil {
		t.Fatalf("list u2: %v", err)
	}
	if len(u2Docs) != 1 {
		t.Errorf("u2 list: got %d docs, want 1", len(u2Docs))
	}
}

func TestListDocuments_TagFilter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	for _, d := range []domain.Document{
		{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}},
		{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Tags: []string{"go"}},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.ListDocuments{Docs: docs}
	got, err := uc.Execute(ctx, "u", []string{"go", "tui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", got)
	}
}
