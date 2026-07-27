package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestRedesignDocTypes_ConvertsAgentDocs(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	seed := func(id string, typ domain.DocumentType, path string) {
		if _, err := store.Create(ctx, domain.Document{
			ID: id, OwnerID: "owner", Type: typ, Path: path,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("1", domain.DocAgent, "plans/2026-x")
	seed("2", domain.DocAgent, "specs/2026-y-design")
	seed("3", domain.DocMemory, "untouched")

	uc := usecase.RedesignDocTypes{Docs: store, Clock: testutil.FakeClock{T: now}}

	// dry-run: reports correct counts but mutates nothing
	rep, err := uc.Execute(ctx, "owner", true)
	if err != nil || rep.Scanned != 2 || rep.Converted != 2 {
		t.Fatalf("dry-run rep=%+v err=%v", rep, err)
	}
	d1, _ := store.Get(ctx, "owner", "1")
	if d1.Type != domain.DocAgent {
		t.Fatalf("dry-run must not mutate: got type %q", d1.Type)
	}

	// real run
	if _, err := uc.Execute(ctx, "owner", false); err != nil {
		t.Fatal(err)
	}
	d1, _ = store.Get(ctx, "owner", "1")
	if d1.Type != domain.DocPlan || d1.Path != "2026-x" {
		t.Errorf("doc 1 = %+v, want plan/2026-x", d1)
	}
	d2, _ := store.Get(ctx, "owner", "2")
	if d2.Type != domain.DocSpec || d2.Path != "2026-y-design" {
		t.Errorf("doc 2 = %+v, want spec/2026-y-design", d2)
	}
	d3, _ := store.Get(ctx, "owner", "3")
	if d3.Type != domain.DocMemory {
		t.Errorf("non-agent doc must be untouched: got %q", d3.Type)
	}

	// idempotent: second run finds nothing
	rep2, _ := uc.Execute(ctx, "owner", false)
	if rep2.Scanned != 0 {
		t.Errorf("second run should find 0 agent docs, got %d", rep2.Scanned)
	}
}
