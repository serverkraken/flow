package modal_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/modal"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRender_DefaultBox(t *testing.T) {
	t.Parallel()
	out := modal.Render("Are you sure?", modal.Opts{Title: "Bestätigen"}, theme.TokyonightNight)
	if !strings.Contains(out, "Are you sure?") {
		t.Errorf("missing content in %q", out)
	}
	if !strings.Contains(out, "Bestätigen") {
		t.Errorf("missing title in %q", out)
	}
	// DoubleBorder should be rendered (top + bottom border lines
	// contain ╔ / ╝ corner glyphs).
	if !strings.Contains(out, "╔") || !strings.Contains(out, "╝") {
		t.Errorf("expected DoubleBorder corners in %q", out)
	}
}

func TestRender_DangerKind(t *testing.T) {
	t.Parallel()
	out := modal.Render("Delete forever?", modal.Opts{Kind: modal.KindDanger}, theme.TokyonightNight)
	if !strings.Contains(out, "Delete forever?") {
		t.Errorf("missing content in %q", out)
	}
}

func TestRender_SafeKind(t *testing.T) {
	t.Parallel()
	out := modal.Render("All set.", modal.Opts{Title: "Fertig", Kind: modal.KindSafe}, theme.TokyonightNight)
	if !strings.Contains(out, "All set.") {
		t.Errorf("missing content in %q", out)
	}
	if !strings.Contains(out, "Fertig") {
		t.Errorf("missing title in %q", out)
	}
}

func TestRender_FixedWidth(t *testing.T) {
	t.Parallel()
	// Smoke: width > content forces padding on the right; the assertion
	// is conservative — content present, no panic.
	out := modal.Render("hi", modal.Opts{Width: 40}, theme.TokyonightNight)
	if !strings.Contains(out, "hi") {
		t.Errorf("missing content in %q", out)
	}
}
