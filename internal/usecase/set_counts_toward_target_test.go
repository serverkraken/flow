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

// TestSetCountsTowardTarget pins the always-apply semantics: unlike
// UpdateNodeInput (nil = preserve), this usecase writes the mode verbatim so
// the WebUI tri-state control can express "set back to inherit" (nil).
func TestSetCountsTowardTarget(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, err := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindRepo
	tt := true
	n.CountsTowardTarget = &tt // start explicit Work
	if _, err := ns.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	uc := usecase.SetCountsTowardTarget{Nodes: ns, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "n1", nil) // set to inherit
	if err != nil {
		t.Fatal(err)
	}
	if got.CountsTowardTarget != nil {
		t.Errorf("expected nil (inherit), got %v", *got.CountsTowardTarget)
	}

	pv := false
	got2, err := uc.Execute(ctx, "u1", "n1", &pv) // set Privat
	if err != nil {
		t.Fatal(err)
	}
	if got2.CountsTowardTarget == nil || *got2.CountsTowardTarget != false {
		t.Errorf("expected explicit false (Privat), got %v", got2.CountsTowardTarget)
	}

	// unknown node → ErrNodeNotFound bubbles (404 at the handler)
	if _, err := uc.Execute(ctx, "u1", "nope", nil); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("unknown node: want ErrNodeNotFound, got %v", err)
	}
}
