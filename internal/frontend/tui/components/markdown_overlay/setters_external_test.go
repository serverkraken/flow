package markdown_overlay_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Setter + SetPalette + writeClipboardCmd coverage. The existing
// model_test.go suite focuses on Update / View; these target the
// surface-mutation API.

func TestSetTitle_SwapsChromeTitle(t *testing.T) {
	render := func(src string, _ int) string { return src }
	m := markdown_overlay.New(
		render,
		markdown_overlay.WithTitle("first"),
		markdown_overlay.WithSource("hello"),
	).SetSize(40, 10)
	if v := m.View(); !strings.Contains(v, "first") {
		t.Errorf("initial title missing from View, got:\n%s", v)
	}
	m = m.SetTitle("second")
	if v := m.View(); !strings.Contains(v, "second") {
		t.Errorf("after SetTitle View should show new title, got:\n%s", v)
	}
}

func TestSetError_DisplacesBodyWithBanner(t *testing.T) {
	render := func(src string, _ int) string { return "BODY:" + src }
	m := markdown_overlay.New(
		render,
		markdown_overlay.WithSource("hi"),
	).SetSize(40, 10)
	// Now inject an error — View should show the err banner rather than the body.
	m = m.SetError(injErr{})
	v := m.View()
	if !strings.Contains(v, "boom") {
		t.Errorf("SetError banner should include error message, got:\n%s", v)
	}
	// SetSource clears the error and re-renders the body.
	m = m.SetSource("recovered")
	v = m.View()
	if !strings.Contains(v, "recovered") {
		t.Errorf("after SetSource the body should render again, got:\n%s", v)
	}
}

type injErr struct{}

func (injErr) Error() string { return "boom" }

// SetPalette installs the Storm/Night palette atomically. The init()
// already seeds theme.Default; SetPalette should not panic and a
// subsequent View() must remain renderable.
func TestSetPalette_ReplacesStylesWithoutPanic(t *testing.T) {
	markdown_overlay.SetPalette(theme.Load())
	render := func(src string, _ int) string { return src }
	m := markdown_overlay.New(
		render,
		markdown_overlay.WithTitle("hello"),
		markdown_overlay.WithSource("body"),
	).SetSize(50, 12)
	if v := m.View(); v == "" {
		t.Errorf("View after SetPalette should still render")
	}
}
