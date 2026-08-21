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

func TestAssignHighlight(t *testing.T) {
	ctx := context.Background()
	hs := testutil.NewFakeNodeHighlightStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	uc := usecase.AssignHighlight{Highlights: hs, IDs: &testutil.FakeIDGen{}, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "doc1", "n1", "  eine markierte Stelle  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Quote != "eine markierte Stelle" {
		t.Errorf("Quote=%q — umgebende Leerzeichen gehören nicht in das Zitat", got.Quote)
	}
	if got.ID == "" || !got.CreatedAt.Equal(clk.T) {
		t.Errorf("highlight not stamped: %+v", got)
	}

	// A passage without text is not a passage.
	if _, err := uc.Execute(ctx, "u1", "doc1", "n1", "   "); !errors.Is(err, domain.ErrInvalidHighlight) {
		t.Errorf("blank quote: want ErrInvalidHighlight, got %v", err)
	}
}

func TestListNewestHighlights_NormalisesLimit(t *testing.T) {
	ctx := context.Background()
	hs := testutil.NewFakeNodeHighlightStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	assign := usecase.AssignHighlight{Highlights: hs, IDs: &testutil.FakeIDGen{}, Clock: clk}
	for _, q := range []string{"eins", "zwei", "drei"} {
		if _, err := assign.Execute(ctx, "u1", "doc1", "n1", q); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.ListNewestHighlights{Highlights: hs}

	// A miswired caller passing 0 must not pull the whole table.
	got, err := uc.Execute(ctx, "u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("limit 0 must normalise to 1, got %d rows", len(got))
	}
	if got, err := uc.Execute(ctx, "u1", 2); err != nil || len(got) != 2 {
		t.Errorf("limit 2 = %d rows, err=%v", len(got), err)
	}
}

func TestRemoveAndListDocumentHighlights(t *testing.T) {
	ctx := context.Background()
	hs := testutil.NewFakeNodeHighlightStore()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)}
	assign := usecase.AssignHighlight{Highlights: hs, IDs: &testutil.FakeIDGen{}, Clock: clk}
	h, err := assign.Execute(ctx, "u1", "doc1", "n1", "eine Stelle")
	if err != nil {
		t.Fatal(err)
	}

	list := usecase.ListDocumentHighlights{Highlights: hs}
	if got, err := list.Execute(ctx, "u1", "doc1"); err != nil || len(got) != 1 {
		t.Fatalf("list = %+v err=%v, want one highlight", got, err)
	}
	// A foreign owner sees nothing and removes nothing.
	if got, err := list.Execute(ctx, "u-fremd", "doc1"); err != nil || len(got) != 0 {
		t.Errorf("foreign list = %+v err=%v, want empty", got, err)
	}
	rm := usecase.RemoveHighlight{Highlights: hs}
	if err := rm.Execute(ctx, "u-fremd", h.ID); !errors.Is(err, ports.ErrNodeHighlightNotFound) {
		t.Errorf("foreign remove: want ErrNodeHighlightNotFound, got %v", err)
	}
	if err := rm.Execute(ctx, "u1", h.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := list.Execute(ctx, "u1", "doc1"); len(got) != 0 {
		t.Errorf("after remove: %+v, want empty", got)
	}
}
