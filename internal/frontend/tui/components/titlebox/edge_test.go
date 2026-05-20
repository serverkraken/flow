package titlebox_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
)

// Edge branches in Render. titlebox_test.go covers the canonical
// happy path; these cover the small-width degradations.

func TestRender_BelowMinimumWidth_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	if out := titlebox.Render("T", "x", 3, testPalette); out != "" {
		t.Errorf("width<4 should return empty, got %q", out)
	}
}

func TestRender_NarrowWidth_FallsBackToTitlelessTop(t *testing.T) {
	t.Parallel()
	// width=6 → titleBudget = 6-6 = 0 → title-less top border branch.
	out := titlebox.Render("Hello", "body", 6, testPalette)
	if out == "" {
		t.Errorf("width=6 should still render (just without title in top)")
	}
}

func TestRender_LongTitleDashesClampToOne(t *testing.T) {
	t.Parallel()
	// A title that consumes the full title budget triggers the
	// dashes<1 → dashes=1 clamp branch.
	out := titlebox.Render("AAAAAAAAAAAAAAAA", "body", 20, testPalette)
	if out == "" {
		t.Errorf("title fully consuming the budget should still render")
	}
}
