package markdown_test

// Tests for the option-less Renderer wrapper. The package-level Render
// function has a full suite already; here we only verify the Renderer
// shim delegates correctly and that WithPalette propagates through to
// the resolved options.

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestNewRenderer_ZeroValueOK(t *testing.T) {
	t.Parallel()
	r := markdown.NewRenderer()
	out, err := r.Render("# Hi\n\nbody", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Hi") {
		t.Errorf("rendered output should contain heading text, got %q", out)
	}
}

func TestRenderer_ZeroWidthReturnsEmpty(t *testing.T) {
	t.Parallel()
	r := markdown.NewRenderer()
	out, _ := r.Render("# Hi", 0)
	if out != "" {
		t.Errorf("width=0 should return empty, got %q", out)
	}
}

func TestRender_WithPaletteOption(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("# Heading\n\nbody", 60, markdown.WithPalette(theme.Load()))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Heading") {
		t.Errorf("output should contain heading, got:\n%s", out)
	}
}
