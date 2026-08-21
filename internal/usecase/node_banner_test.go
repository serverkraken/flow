package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestValidateNodeBanner(t *testing.T) {
	t.Parallel()
	mime, err := usecase.ValidateNodeBanner(pngPixel(t))
	if err != nil || mime != "image/png" {
		t.Errorf("valid PNG: got (%q, %v), want (image/png, nil)", mime, err)
	}
	if _, err := usecase.ValidateNodeBanner([]byte("nope, plain text")); !errors.Is(err, usecase.ErrBannerBadType) {
		t.Errorf("text upload: want ErrBannerBadType, got %v", err)
	}
	// A PNG header the decoder cannot actually read must not slip through on
	// the sniff alone.
	truncated := pngPixel(t)[:20]
	if _, err := usecase.ValidateNodeBanner(truncated); !errors.Is(err, usecase.ErrBannerBadType) {
		t.Errorf("truncated PNG: want ErrBannerBadType, got %v", err)
	}
	// The banner is a wide strip, so it is allowed to be larger than a logo —
	// but not unbounded.
	if usecase.MaxNodeBannerBytes <= usecase.MaxNodeLogoBytes {
		t.Errorf("MaxNodeBannerBytes=%d must exceed MaxNodeLogoBytes=%d", usecase.MaxNodeBannerBytes, usecase.MaxNodeLogoBytes)
	}
	oversized := append(pngPixel(t), bytes.Repeat([]byte{0}, usecase.MaxNodeBannerBytes)...)
	if _, err := usecase.ValidateNodeBanner(oversized); !errors.Is(err, usecase.ErrBannerTooLarge) {
		t.Errorf("oversized upload: want ErrBannerTooLarge, got %v", err)
	}
}

// TestGetNodeBanner covers the serving path: the blob comes back byte for
// byte, and a foreign owner sees nothing.
func TestGetNodeBanner(t *testing.T) {
	ctx := context.Background()
	bs := testutil.NewFakeNodeBannerStore()
	if err := bs.Put(ctx, domain.NodeBanner{
		NodeID: "n1", OwnerID: "u1", Mime: "image/png", Ref: "abc123abc123",
		Bytes: pngPixel(t), UpdatedAt: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatal(err)
	}
	uc := usecase.GetNodeBanner{Banners: bs}

	got, err := uc.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes, pngPixel(t)) {
		t.Errorf("served bytes differ from the stored ones")
	}
	if _, err := uc.Execute(ctx, "u-fremd", "n1"); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("foreign get: want ErrNodeBannerNotFound, got %v", err)
	}
}

// TestUpdateNode_BannerRidesTheAggregate pins that the edit form's ONE submit
// carries the banner inside the same transaction as name, tags and rate. A
// banner written beside the aggregate would be clobbered by the metadata
// write that reads the node before the blob lands.
func TestUpdateNode_BannerRidesTheAggregate(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, testutil.NewFakeNodeLogoStore(), bs, testutil.NewFakeTagStore())
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UpdateNode{Nodes: ns, Aggregate: agg, Clock: clk}

	name := "flow neu"
	got, err := uc.Execute(ctx, "u1", "n1", usecase.UpdateNodeInput{Name: &name, BannerData: pngPixel(t)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "flow neu" {
		t.Errorf("Name=%q want the renamed node", got.Name)
	}
	if len(got.BannerRef) != 12 {
		t.Errorf("BannerRef=%q want the content hash", got.BannerRef)
	}
	stored, err := bs.Get(ctx, "u1", "n1")
	if err != nil || stored.Ref != got.BannerRef {
		t.Fatalf("blob and ref diverged: %+v err=%v", stored, err)
	}

	// A metadata-only update must leave the banner alone.
	name2 := "flow noch neuer"
	kept, err := uc.Execute(ctx, "u1", "n1", usecase.UpdateNodeInput{Name: &name2})
	if err != nil {
		t.Fatal(err)
	}
	if kept.BannerRef != got.BannerRef {
		t.Errorf("metadata-only update lost the banner: %q → %q", got.BannerRef, kept.BannerRef)
	}

	// Removing it clears ref and blob together.
	cleared, err := uc.Execute(ctx, "u1", "n1", usecase.UpdateNodeInput{DeleteBanner: true})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.BannerRef != "" {
		t.Errorf("BannerRef=%q want cleared", cleared.BannerRef)
	}
	if _, err := bs.Get(ctx, "u1", "n1"); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("blob outlived the ref: %v", err)
	}

	// Upload and delete in one submit is a caller mistake, not a silent winner.
	if _, err := uc.Execute(ctx, "u1", "n1", usecase.UpdateNodeInput{BannerData: pngPixel(t), DeleteBanner: true}); err == nil {
		t.Error("upload+delete together must be rejected")
	}
}

// TestCreateNode_BannerRidesTheAggregate is the create-side twin: a register
// created with a banner gets ref and blob in the same transaction, exactly
// like the logo.
func TestCreateNode_BannerRidesTheAggregate(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, testutil.NewFakeNodeLogoStore(), bs, testutil.NewFakeTagStore())
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	uc := usecase.CreateNode{Nodes: ns, Aggregate: agg, IDs: &testutil.FakeIDGen{}, Clock: clk}

	got, err := uc.Execute(ctx, "u1", usecase.CreateNodeInput{
		Name: "flow", Kind: domain.KindEngagement, BannerData: pngPixel(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BannerRef) != 12 {
		t.Fatalf("BannerRef=%q want the content hash", got.BannerRef)
	}
	stored, err := bs.Get(ctx, "u1", got.ID)
	if err != nil {
		t.Fatalf("blob missing: %v", err)
	}
	if stored.Ref != got.BannerRef || stored.NodeID != got.ID {
		t.Errorf("blob does not belong to the created node: %+v vs %+v", stored, got)
	}
}

// TestUploadNodeBanner_AgentPath covers the agent-facing upload (REST/MCP):
// it goes through the aggregate too, so ref and blob land together, and it
// refuses a node that is not the caller's.
func TestUploadNodeBanner_AgentPath(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, testutil.NewFakeNodeLogoStore(), bs, testutil.NewFakeTagStore())
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UploadNodeBanner{Nodes: ns, Banners: bs, Aggregate: agg, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "n1", pngPixel(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BannerRef) != 12 {
		t.Errorf("BannerRef=%q want the content hash", got.BannerRef)
	}
	stored, err := bs.Get(ctx, "u1", "n1")
	if err != nil || stored.Ref != got.BannerRef {
		t.Fatalf("blob and ref diverged: %+v err=%v", stored, err)
	}
	if _, err := uc.Execute(ctx, "u-fremd", "n1", pngPixel(t)); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("foreign upload: want ErrNodeNotFound, got %v", err)
	}
	if _, err := uc.Execute(ctx, "u1", "n1", []byte("kein Bild")); !errors.Is(err, usecase.ErrBannerBadType) {
		t.Errorf("non-image: want ErrBannerBadType, got %v", err)
	}
}
