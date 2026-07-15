package usecase_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// pngPixel is a valid 1x1 PNG (sniffs as image/png).
func pngPixel(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUploadNodeLogo(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "n1", pngPixel(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.LogoRef) != 12 {
		t.Errorf("LogoRef = %q, want 12-hex content hash", got.LogoRef)
	}
	logo, err := ls.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if logo.Mime != "image/png" || logo.Ref != got.LogoRef || len(logo.Bytes) == 0 {
		t.Errorf("stored logo mime=%q ref=%q len=%d", logo.Mime, logo.Ref, len(logo.Bytes))
	}

	// Rejections: wrong type, oversized, unknown node.
	if _, err := uc.Execute(ctx, "u1", "n1", []byte("plain text, not an image")); !errors.Is(err, usecase.ErrLogoBadType) {
		t.Errorf("text upload → %v, want ErrLogoBadType", err)
	}
	if _, err := uc.Execute(ctx, "u1", "n1", make([]byte, usecase.MaxNodeLogoBytes+1)); !errors.Is(err, usecase.ErrLogoTooLarge) {
		t.Errorf("oversized upload → %v, want ErrLogoTooLarge", err)
	}
	if _, err := uc.Execute(ctx, "u1", "ghost", pngPixel(t)); err == nil {
		t.Error("unknown node accepted")
	}
}

func TestDeleteNodeLogo(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	up := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk}
	if _, err := up.Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}
	del := usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Clock: clk}
	got, err := del.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogoRef != "" {
		t.Errorf("LogoRef = %q after delete, want empty", got.LogoRef)
	}
	if _, err := ls.Get(ctx, "u1", "n1"); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("blob still present after delete: %v", err)
	}
	// Deleting again is a no-op, not an error.
	if _, err := del.Execute(ctx, "u1", "n1"); err != nil {
		t.Errorf("second delete errored: %v", err)
	}
}

func TestUploadNodeLogo_RollsBackBlobWhenNodeWriteFails(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, ls, tags)
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	agg.FailStage = testutil.NodeAggregateFailNode

	uc := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk}
	if _, err := uc.Execute(ctx, "u1", "n1", pngPixel(t)); err == nil {
		t.Fatal("upload succeeded despite injected node write failure")
	}
	got, err := ns.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogoRef != "" {
		t.Fatalf("logo ref persisted after rollback: %q", got.LogoRef)
	}
	if _, err := ls.Get(ctx, "u1", "n1"); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Fatalf("logo blob persisted after rollback: %v", err)
	}
}

func TestDeleteNodeLogo_RollsBackRefWhenBlobDeleteFails(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, ls, tags)
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	upload := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk}
	before, err := upload.Execute(ctx, "u1", "n1", pngPixel(t))
	if err != nil {
		t.Fatal(err)
	}
	agg.FailStage = testutil.NodeAggregateFailLogo

	del := usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk}
	if _, err := del.Execute(ctx, "u1", "n1"); err == nil {
		t.Fatal("delete succeeded despite injected blob failure")
	}
	got, err := ns.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogoRef != before.LogoRef {
		t.Fatalf("logo ref changed after rollback: got %q want %q", got.LogoRef, before.LogoRef)
	}
	logo, err := ls.Get(ctx, "u1", "n1")
	if err != nil || logo.Ref != before.LogoRef {
		t.Fatalf("logo blob changed after rollback: logo=%+v err=%v", logo, err)
	}
}

func TestDeleteNodeLogo_RemovesOrphanBlobWhenRefIsAlreadyEmpty(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, ls, tags)
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	if err := ls.Put(ctx, domain.NodeLogo{NodeID: n.ID, OwnerID: n.OwnerID, Ref: "orphan", Bytes: pngPixel(t)}); err != nil {
		t.Fatal(err)
	}

	del := usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk}
	if _, err := del.Execute(ctx, "u1", n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.Get(ctx, "u1", n.ID); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Fatalf("orphan blob survived delete: %v", err)
	}
}

// TestUploadNodeLogo_MeasuresDimensions pins that ValidateNodeLogo's measured
// width/height (via image.DecodeConfig) land on the stored NodeLogo — the
// aspect ratio webui.LogoShape later needs to pick hex vs. tile treatment.
func TestUploadNodeLogo_MeasuresDimensions(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk}

	if _, err := uc.Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}
	logo, err := ls.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if logo.Width != 1 || logo.Height != 1 {
		t.Errorf("stored dims = %dx%d, want 1x1", logo.Width, logo.Height)
	}
}

// TestGetNodeLogo_LazyBackfillDimensions pins the migration-0027 backfill
// path: a legacy row (Width==0/Height==0, uploaded before dimensions were
// measured) gets its dimensions measured from Bytes on first Get, the
// returned value carries them, and the store is updated best-effort so a
// later Get doesn't need to re-measure.
func TestGetNodeLogo_LazyBackfillDimensions(t *testing.T) {
	ctx := context.Background()
	ls := testutil.NewFakeNodeLogoStore()
	legacy := domain.NodeLogo{
		NodeID: "n1", OwnerID: "u1", Mime: "image/png", Ref: "aaaabbbbcccc",
		Bytes: pngPixel(t), UpdatedAt: time.Now(),
		// Width/Height left at the zero value: pre-migration-0027 row.
	}
	if err := ls.Put(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	uc := usecase.GetNodeLogo{Logos: ls}

	got, err := uc.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 1 || got.Height != 1 {
		t.Errorf("Execute() dims = %dx%d, want 1x1 (measured)", got.Width, got.Height)
	}
	stored, err := ls.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Width != 1 || stored.Height != 1 {
		t.Errorf("store not backfilled: dims = %dx%d, want 1x1", stored.Width, stored.Height)
	}
}
