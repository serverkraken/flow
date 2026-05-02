package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestMain forces lipgloss's default renderer onto a TrueColor
// profile so SGR-presence assertions work without a TTY. Without
// this go test detects no terminal and downgrades the renderer to
// Ascii — which would make every "expected SGR sequence" assertion
// fail for the wrong reason.
func TestMain(m *testing.M) {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// TestRender_ZeroWidthReturnsEmpty: width <= 0 short-circuits to "".
// Callers (browse preview pane on a too-narrow terminal) rely on this
// — they would otherwise feed cellbuf an unworkable wrap budget.
func TestRender_ZeroWidthReturnsEmpty(t *testing.T) {
	t.Parallel()
	out, err := Render("# heading", 0)
	if err != nil {
		t.Fatalf("Render(width=0): %v", err)
	}
	if out != "" {
		t.Errorf("want empty, got %q", out)
	}
}

// TestRender_H1IsBanner: H1 renders as a single full-width row
// carrying the heading text on the banner background. Width must
// equal r.width so consecutive H1s line up.
func TestRender_H1IsBanner(t *testing.T) {
	t.Parallel()
	out, err := Render("# Hello", 40)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Hello") {
		t.Errorf("banner missing heading text: %q", plain)
	}
	bannerLine := ""
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "Hello") {
			bannerLine = l
			break
		}
	}
	if bannerLine == "" {
		t.Fatalf("no line containing the heading text:\n%s", plain)
	}
	if w := lipgloss.Width(bannerLine); w != 40 {
		t.Errorf("banner line width = %d, want 40\n%q", w, bannerLine)
	}
}

// TestRender_HeadingsLevelMonotonic: each level produces a distinct
// rendering. We don't pin the exact visual treatment, just assert
// neighbouring levels diverge so the hierarchy never collapses.
func TestRender_HeadingsLevelMonotonic(t *testing.T) {
	t.Parallel()
	prev := ""
	for level := 1; level <= 6; level++ {
		src := strings.Repeat("#", level) + " heading text"
		out, err := Render(src, 60)
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		if out == prev {
			t.Errorf("level %d rendering collapsed onto previous level\n%q", level, out)
		}
		prev = out
	}
}

// TestRender_ParagraphReflowsToWidth: a long paragraph reflows so
// that no line exceeds the width budget. Tests the public API path
// — confirms wrapText is wired into the paragraph renderer.
func TestRender_ParagraphReflowsToWidth(t *testing.T) {
	t.Parallel()
	src := strings.Repeat("word ", 30)
	const width = 30
	out, err := Render(src, width)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, l := range strings.Split(out, "\n") {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("paragraph line exceeds budget %d: width=%d\n%q", width, w, l)
		}
	}
}

// TestRender_HRuleFullWidth: a thematic break renders as a single
// line of `─` that fills the width budget exactly.
func TestRender_HRuleFullWidth(t *testing.T) {
	t.Parallel()
	out, err := Render("---", 40)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, l := range strings.Split(plain, "\n") {
		if strings.ContainsRune(l, '─') {
			if w := lipgloss.Width(l); w != 40 {
				t.Errorf("HR width = %d, want 40\n%q", w, l)
			}
			return
		}
	}
	t.Errorf("no rule line found in output:\n%s", plain)
}

// TestRender_StrongAndEmphasisStyled: **bold** and *italic* leave
// SGR sequences in the output. Stripping ANSI must yield the bare
// text so search-on-rendered keeps working.
func TestRender_StrongAndEmphasisStyled(t *testing.T) {
	t.Parallel()
	out, err := Render("text with **bold** and *italic* parts", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected SGR sequences in styled output\n%q", out)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "bold") || !strings.Contains(plain, "italic") {
		t.Errorf("plain text lost styled words: %q", plain)
	}
}

// TestRender_CodeSpanCarriesBg: inline `code` carries a background
// SGR. We don't pin the exact colour code; just assert *some* BG SGR
// (`48;`) appears in the output.
func TestRender_CodeSpanCarriesBg(t *testing.T) {
	t.Parallel()
	out, err := Render("a `snippet` of code", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("expected SGR around inline code")
	}
	if !strings.Contains(out, "48;") {
		t.Errorf("expected a background SGR (48;…) around inline code\n%q", out)
	}
}

// TestRender_NoColorStripsSGR: WithNoColor produces output that
// ansi.Strip leaves unchanged — i.e. no SGR sequences at all. The
// glyph + whitespace hierarchy still has to convey structure, so
// the H1 banner's ` ` row text must survive even without colour.
func TestRender_NoColorStripsSGR(t *testing.T) {
	t.Parallel()
	src := "# Title\n\nbody text\n"
	out, err := Render(src, 40, WithNoColor(true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if ansi.Strip(out) != out {
		t.Errorf("WithNoColor output still contains SGR sequences\n%q", out)
	}
	if !strings.Contains(out, "Title") || !strings.Contains(out, "body text") {
		t.Errorf("WithNoColor output dropped content:\n%q", out)
	}
}

// TestRender_WrapsURLsAsOSC8: bare http(s) URLs in prose come out
// wrapped in OSC 8 — opener carries an id= stamp (so multi-line
// wraps stay one click target) and the closer is the standard
// `]8;;`. URL handling now goes through the AutoLink renderer (GFM
// linkify produces an AutoLink for bare URLs), not WrapURLs.
func TestRender_WrapsURLsAsOSC8(t *testing.T) {
	t.Parallel()
	const url = "https://example.com/x"
	out, err := Render("see "+url+" today", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b]8;id=") {
		t.Errorf("missing OSC 8 opener with id= for %q\n%q", url, out)
	}
	if !strings.Contains(out, ";"+url+"\x07") {
		t.Errorf("URL %q missing from OSC 8 destination\n%q", url, out)
	}
	if !strings.Contains(out, "\x1b]8;;\x07") {
		t.Errorf("missing OSC 8 closer\n%q", out)
	}
}

// TestRender_FallbackPreservesContent: nodes whose rich renderer
// hasn't landed yet (lists, fenced code, blockquote, table in
// P1.11.0) must still surface their text content via the walk-only
// fallback — otherwise notes go silent for that block during the
// phase rollout.
func TestRender_FallbackPreservesContent(t *testing.T) {
	t.Parallel()
	src := "- one\n- two\n\n```go\nfunc main() {}\n```\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"one", "two", "func main"} {
		if !strings.Contains(plain, want) {
			t.Errorf("fallback dropped content %q from output:\n%s", want, plain)
		}
	}
}

// TestRender_MarkdownLinkEmitsTextAndURL: a `[text](url)` link
// renders the text styled, with the URL stowed in the OSC 8
// hyperlink (not visible in the strip-ANSI form). The destination
// shouldn't take up horizontal space — terminals reveal it on hover.
func TestRender_MarkdownLinkEmitsTextAndURL(t *testing.T) {
	t.Parallel()
	out, err := Render("see [docs](https://example.com)", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "docs") {
		t.Errorf("link text missing: %q", plain)
	}
	if !strings.Contains(out, ";https://example.com\x07") {
		t.Errorf("OSC 8 destination missing for markdown link\n%q", out)
	}
}

// TestRender_ImageEmitsChip: `![alt](url)` becomes a `[image: alt — url]`
// chip — no graphics protocol in P1.11.0, just a textual placeholder.
func TestRender_ImageEmitsChip(t *testing.T) {
	t.Parallel()
	out, err := Render("![diagram](https://example.com/d.png)", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "[image: diagram") {
		t.Errorf("image chip missing 'image:' prefix: %q", plain)
	}
	if !strings.Contains(plain, "https://example.com/d.png") {
		t.Errorf("image chip missing destination: %q", plain)
	}
}

// TestRender_InlineHTMLPreserved: raw inline HTML survives via the
// renderRawInline fallback. Glamour today renders the angle brackets
// as plain text; the new pipeline matches.
func TestRender_InlineHTMLPreserved(t *testing.T) {
	t.Parallel()
	out, err := Render("text <span>inner</span> tail", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "<span>") || !strings.Contains(plain, "</span>") {
		t.Errorf("raw inline HTML lost from output: %q", plain)
	}
	if !strings.Contains(plain, "inner") {
		t.Errorf("inline HTML inner text dropped: %q", plain)
	}
}

// TestRender_HTMLBlockPreserved: a raw HTML block (CommonMark
// recognises a `<div>...</div>` paragraph as a block-level HTML
// element) round-trips through the renderRawBlock fallback so the
// reader sees the markup instead of nothing.
func TestRender_HTMLBlockPreserved(t *testing.T) {
	t.Parallel()
	src := "<div>\n  raw html block\n</div>\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "<div>") || !strings.Contains(plain, "</div>") {
		t.Errorf("HTML block tags lost from output:\n%s", plain)
	}
	if !strings.Contains(plain, "raw html block") {
		t.Errorf("HTML block content lost:\n%s", plain)
	}
}

// TestRender_BlockquoteFallbackPreservesContent: the walk-only
// fallback registered for Blockquote in P1.11.0 must surface the
// quoted prose. Real bar styling lands in P1.11.7 (callout work).
func TestRender_BlockquoteFallbackPreservesContent(t *testing.T) {
	t.Parallel()
	out, err := Render("> a quoted line\n> and another", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"quoted line", "another"} {
		if !strings.Contains(plain, want) {
			t.Errorf("blockquote fallback dropped %q from output:\n%s", want, plain)
		}
	}
}

// TestOptions_FunctionalSettersWire confirms the With… helpers
// actually mutate the options struct. Asserts a passed-in
// frontmatter / backlinks / wikilinks / nerdfont value lands in
// the resolved options after buildOptions runs.
func TestOptions_FunctionalSettersWire(t *testing.T) {
	t.Parallel()
	fm := struct{}{} // placeholder — real type is domain.Frontmatter
	_ = fm
	res := stubResolver{}
	o := buildOptions([]Option{
		WithWikilinks(res),
		WithNerdFont(true),
	})
	if o.resolver == nil {
		t.Error("WithWikilinks did not set resolver")
	}
	if !o.nerdFont {
		t.Error("WithNerdFont(true) did not set nerdFont")
	}
}

// TestOptions_FrontmatterAndBacklinksOptional: passing nil
// frontmatter and a nil backlinks slice is the same as omitting
// the option — buildOptions returns zero values rather than
// panicking.
func TestOptions_FrontmatterAndBacklinksOptional(t *testing.T) {
	t.Parallel()
	o := buildOptions([]Option{
		WithFrontmatter(nil),
		WithBacklinks(nil),
	})
	if o.frontmatter != nil {
		t.Error("expected nil frontmatter")
	}
	if o.backlinks != nil {
		t.Error("expected nil backlinks")
	}
}

type stubResolver struct{}

func (stubResolver) Resolve(string) (string, string, bool) { return "", "", false }
