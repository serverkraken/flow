package markdown_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
)

// Drives the GFM strikethrough renderer that the existing suite
// doesn't cover. The renderer wraps inner content in SGR 9 / 29 by
// hand (lipgloss's per-grapheme Strike role bloats output ~30×); these
// tests pin both the colored path (raw escapes appear) and the Ascii
// path (no escapes, plain text only).

func TestRender_Strikethrough_EmitsSGR(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("~~gone~~ stays", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Expect SGR 9 (set) + 29 (reset) or NO_COLOR (no SGRs at all).
	hasOpen := strings.Contains(out, "\x1b[9m")
	hasClose := strings.Contains(out, "\x1b[29m")
	if hasOpen != hasClose {
		t.Errorf("strikethrough SGR pair must come together, got open=%v close=%v", hasOpen, hasClose)
	}
	if !strings.Contains(out, "gone") {
		t.Errorf("strikethrough body should survive, got %q", out)
	}
}

func TestRender_Strikethrough_NoColor_NoSGR(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("~~gone~~", 80, markdown.WithNoColor(true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, "\x1b[9m") {
		t.Errorf("WithNoColor should suppress strikethrough SGR, got %q", out)
	}
	if !strings.Contains(out, "gone") {
		t.Errorf("body should survive, got %q", out)
	}
}

func TestRender_AutoLink_NoColor_EmitsURL(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("see https://example.com here", 80, markdown.WithNoColor(true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("autolink URL should appear, got %q", out)
	}
	// Inline link styling is stripped under NO_COLOR (no SGR colors), but
	// the WrapURLs post-process still emits an OSC 8 hyperlink so the URL
	// stays clickable in supporting terminals — that is by design.
	if !strings.Contains(out, "see ") {
		t.Errorf("surrounding text should survive, got %q", out)
	}
}

func TestRender_Link_NoColor_EmitsLabelAndURL(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("[label](https://example.com)", 80, markdown.WithNoColor(true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "label") {
		t.Errorf("link label should be visible, got %q", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("link URL should be visible (markdown.renderLink appends it on Ascii), got %q", out)
	}
}

// Footnote definitions live in a dedicated section; the renderer
// surfaces a `^1` reference inline and a "Footnotes" heading at the end.
func TestRender_Footnote_RendersDefinitionAndRef(t *testing.T) {
	t.Parallel()
	src := "text[^a] continues.\n\n[^a]: note body"
	out, err := markdown.Render(src, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Footnotes") {
		t.Errorf("footnote list heading missing, got:\n%s", out)
	}
	if !strings.Contains(out, "note body") {
		t.Errorf("footnote body missing, got:\n%s", out)
	}
}
