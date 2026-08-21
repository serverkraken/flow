package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
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

func TestUploadNodeBanner(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UploadNodeBanner{Nodes: ns, Banners: bs, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "n1", pngPixel(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BannerRef) != 12 {
		t.Errorf("BannerRef=%q want the 12-char content hash", got.BannerRef)
	}
	if strings.ContainsAny(got.BannerRef, "GHIJKLMNOPQRSTUVWXYZ") {
		t.Errorf("BannerRef=%q want lowercase hex", got.BannerRef)
	}
	stored, err := bs.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatalf("banner not stored: %v", err)
	}
	if stored.Ref != got.BannerRef || stored.Mime != "image/png" {
		t.Errorf("stored %+v does not match the node's ref %q", stored, got.BannerRef)
	}

	// A foreign owner must not reach the node at all.
	if _, err := uc.Execute(ctx, "u-fremd", "n1", pngPixel(t)); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("foreign upload: want ErrNodeNotFound, got %v", err)
	}
}

func TestDeleteNodeBanner(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	if _, err := (usecase.UploadNodeBanner{Nodes: ns, Banners: bs, Clock: clk}).
		Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}

	uc := usecase.DeleteNodeBanner{Nodes: ns, Banners: bs, Clock: clk}
	got, err := uc.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.BannerRef != "" {
		t.Errorf("BannerRef=%q want cleared", got.BannerRef)
	}
	if _, err := bs.Get(ctx, "u1", "n1"); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("blob still there: %v", err)
	}
	// Deleting again is a no-op, not an error — the node has nothing to lose.
	if _, err := uc.Execute(ctx, "u1", "n1"); err != nil {
		t.Errorf("second delete: %v", err)
	}
}

func TestGetNodeBanner(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	if _, err := (usecase.UploadNodeBanner{Nodes: ns, Banners: bs, Clock: clk}).
		Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}
	uc := usecase.GetNodeBanner{Banners: bs}

	got, err := uc.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes, pngPixel(t)) {
		t.Errorf("served bytes differ from the uploaded ones")
	}
	if _, err := uc.Execute(ctx, "u-fremd", "n1"); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("foreign get: want ErrNodeBannerNotFound, got %v", err)
	}
}
