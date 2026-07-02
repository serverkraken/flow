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
