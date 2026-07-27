package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStripFrontmatter_MovesBlockToExtra(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Body: "---\ntags: [go]\naliases: [x]\n---\nreal body", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	uc := usecase.StripFrontmatter{Docs: docs, Clock: testutil.FakeClock{T: time.Now()}}

	rep, err := uc.Execute(ctx, "u1", true) // dry-run
	if err != nil || rep.Stripped != 1 {
		t.Fatalf("dry-run want Stripped=1, got %+v err=%v", rep, err)
	}
	d, _ := docs.Get(ctx, "u1", "d1")
	if d.Body != "---\ntags: [go]\naliases: [x]\n---\nreal body" {
		t.Fatalf("dry-run must not mutate, got body %q", d.Body)
	}

	_, _ = uc.Execute(ctx, "u1", false) // real
	d, _ = docs.Get(ctx, "u1", "d1")
	if d.Body != "real body" {
		t.Fatalf("body not stripped: %q", d.Body)
	}
	if d.Extra["frontmatter"] == nil {
		t.Fatalf("frontmatter not preserved into extra")
	}
}
