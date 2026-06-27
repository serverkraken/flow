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

func newImport(docs ports.DocumentStore) usecase.ImportDocument {
	return usecase.ImportDocument{
		Docs:  docs,
		IDs:   &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)},
	}
}

// A daily import keeps its ORIGINAL date and path (no CreateDocument now-override).
func TestImportDocument_DailyKeepsHistoricalDateAndPath(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	d0 := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	got, err := uc.Execute(context.Background(), "owner-1", usecase.ImportDocumentInput{
		Type: domain.DocDaily, Path: "daily/2026-04-28", Title: "2026-04-28",
		Body: "# 2026-04-28\n", Date: &d0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "daily/2026-04-28" {
		t.Fatalf("path overridden: %q", got.Path)
	}
	if got.Date == nil || !got.Date.Equal(d0) {
		t.Fatalf("date not preserved: %v", got.Date)
	}
}

// A project import persists the provided NodeID and parses frontmatter tags.
func TestImportDocument_ProjectAndTags(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	pid := "proj-1"
	got, err := uc.Execute(context.Background(), "owner-1", usecase.ImportDocumentInput{
		Type: domain.DocProject, Path: "projects/foo/readme", Title: "Foo",
		Body: "---\ntags: [infra, gcp]\n---\n# Foo\n", NodeID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeID == nil || *got.NodeID != "proj-1" {
		t.Fatalf("nodeID not persisted: %v", got.NodeID)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "infra" {
		t.Fatalf("tags not parsed from frontmatter: %v", got.Tags)
	}
}

// Re-importing the same path surfaces the store's duplicate error for the caller to skip.
func TestImportDocument_DuplicatePath(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	in := usecase.ImportDocumentInput{Type: domain.DocFree, Path: "notes/onboarding", Title: "O", Body: "x"}
	if _, err := uc.Execute(context.Background(), "owner-1", in); err != nil {
		t.Fatal(err)
	}
	_, err := uc.Execute(context.Background(), "owner-1", in)
	if !errors.Is(err, ports.ErrDocumentExists) {
		t.Fatalf("want ErrDocumentExists on duplicate path, got %v", err)
	}
}
