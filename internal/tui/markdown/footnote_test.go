package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
)

// TestRender_Footnote_RefRendersAsSuperscript: a `[^1]` reference
// shows up as the Unicode superscript ¹ in the body.
func TestRender_Footnote_RefRendersAsSuperscript(t *testing.T) {
	t.Parallel()
	out, err := Render("see ref[^1].\n\n[^1]: text\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "ref¹") {
		t.Errorf("inline footnote ref missing superscript:\n%s", plain)
	}
}

// TestRender_Footnote_DefinitionsListed: every definition appears
// in a "Footnotes" section at the end of the document, prefixed
// with its superscript marker.
func TestRender_Footnote_DefinitionsListed(t *testing.T) {
	t.Parallel()
	src := "first[^a] second[^b]\n\n[^a]: alpha def\n[^b]: bravo def\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Footnotes") {
		t.Errorf("Footnotes heading missing:\n%s", plain)
	}
	if !strings.Contains(plain, "¹ alpha def") {
		t.Errorf("alpha definition with marker missing:\n%s", plain)
	}
	if !strings.Contains(plain, "² bravo def") {
		t.Errorf("bravo definition with marker missing:\n%s", plain)
	}
}

// TestRender_Footnote_BacklinkSwallowed: goldmark inserts a
// FootnoteBacklink inline in each definition. We deliberately don't
// render it — in the TUI there's no jump-back affordance, only
// noise. Asserts no `↩` glyph (or similar) leaks through.
func TestRender_Footnote_BacklinkSwallowed(t *testing.T) {
	t.Parallel()
	out, err := Render("ref[^1]\n\n[^1]: def\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, glyph := range []string{"↩", "&#8617;", "&#x21a9;"} {
		if strings.Contains(plain, glyph) {
			t.Errorf("backlink glyph %q leaked into output:\n%s", glyph, plain)
		}
	}
}

// TestRender_Footnote_RegisteredKindsCovered: the renderFootnote
// + renderFootnoteBacklink no-op handlers exist mainly to swallow
// kinds that goldmark would otherwise dispatch to its bundled HTML
// renderer. We exercise them through an end-to-end render so the
// dispatch table entry is exercised even though the handlers
// themselves emit nothing.
func TestRender_Footnote_RegisteredKindsCovered(t *testing.T) {
	t.Parallel()
	out, err := Render("see[^x]\n\n[^x]: text body\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(ansi.Strip(out), "text body") {
		t.Errorf("definition body missing — renderFootnote dispatch broken:\n%s", ansi.Strip(out))
	}
}

// TestSuperscript_DigitMapping: every 0-9 digit maps to the
// matching Unicode superscript codepoint.
func TestSuperscript_DigitMapping(t *testing.T) {
	t.Parallel()
	cases := map[int]string{1: "¹", 2: "²", 5: "⁵", 10: "¹⁰", 42: "⁴²"}
	for n, want := range cases {
		if got := superscript(n); got != want {
			t.Errorf("superscript(%d) = %q, want %q", n, got, want)
		}
	}
	if got := superscript(0); got == "" {
		t.Error("superscript(0) should fall back to plain marker, got empty")
	}
}

// TestRenderFootnote_DirectCall exercises renderFootnote and
// renderFootnoteBacklink directly (both at 0% coverage). These no-op handlers
// suppress the HTML fallback renderer for goldmark's Footnote and
// FootnoteBacklink AST node kinds. Since they return static values they are
// trivially safe to call with nil arguments.
func TestRenderFootnote_DirectCall(t *testing.T) {
	t.Parallel()
	r := &nodeRenderer{}
	status, err := r.renderFootnote(nil, nil, nil, false)
	if err != nil {
		t.Fatalf("renderFootnote returned error: %v", err)
	}
	if status != ast.WalkSkipChildren {
		t.Errorf("renderFootnote status = %v, want WalkSkipChildren", status)
	}
	status2, err2 := r.renderFootnoteBacklink(nil, nil, nil, false)
	if err2 != nil {
		t.Fatalf("renderFootnoteBacklink returned error: %v", err2)
	}
	if status2 != ast.WalkSkipChildren {
		t.Errorf("renderFootnoteBacklink status = %v, want WalkSkipChildren", status2)
	}
}
