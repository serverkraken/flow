package webui

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

// mustHTML discards DocMeta so call sites can inline
// string(mustHTML(RenderDocument(...))) for tests that don't care about it.
func mustHTML(h template.HTML, _ DocMeta) template.HTML { return h }

// nilResolve is a WikilinkResolver that never resolves — an alias of
// resolveNone kept under the name the Task-2 brief's tests use verbatim.
func nilResolve(target string) (string, string, bool) { return resolveNone(target) }

func TestRenderMarkdown_Basic(t *testing.T) {
	got := string(RenderMarkdown("# Title\n\nsome **bold** text"))
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "<strong>bold</strong>") {
		t.Errorf("markdown not rendered: %q", got)
	}
}

func TestRenderMarkdown_SanitizesScript(t *testing.T) {
	got := string(RenderMarkdown("hi\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1))"))
	if strings.Contains(got, "<script>") || strings.Contains(got, "javascript:") {
		t.Errorf("unsafe content not sanitized: %q", got)
	}
}

func TestRenderMarkdown_SkipsFrontmatter(t *testing.T) {
	html := string(RenderMarkdown("---\ntags: [go]\n---\n# Hello\n"))
	if strings.Contains(html, "tags:") {
		t.Fatalf("frontmatter leaked into output: %q", html)
	}
	if !strings.Contains(html, "<h1") {
		t.Fatalf("body heading missing: %q", html)
	}
}

func resolveNone(target string) (string, string, bool) { return "", "", false }

func TestRenderDocument_GFMTable(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "| A | B |\n|---|---|\n| 1 | 2 |\n", resolveNone)))
	if !strings.Contains(out, "<table") || !strings.Contains(out, "<td") {
		t.Fatalf("expected GFM table, got: %s", out)
	}
}

func TestRenderDocument_Tasklist(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "- [x] done\n- [ ] todo\n", resolveNone)))
	if !strings.Contains(out, `type="checkbox"`) {
		t.Fatalf("expected task checkboxes, got: %s", out)
	}
}

func TestRenderDocument_Strikethrough(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "~~gone~~\n", resolveNone)))
	if !strings.Contains(out, "<del>") {
		t.Fatalf("expected <del>, got: %s", out)
	}
}

func TestRenderDocument_Footnote(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "Text[^1]\n\n[^1]: note\n", resolveNone)))
	if !strings.Contains(out, `class="footnotes"`) && !strings.Contains(out, "footnote-ref") {
		t.Fatalf("expected footnote markup, got: %s", out)
	}
}

func TestRenderDocument_XSSStripped(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "<script>alert(1)</script>\n\n[ok](javascript:alert(1))\n", resolveNone)))
	if strings.Contains(out, "<script") || strings.Contains(out, "javascript:") {
		t.Fatalf("XSS not stripped: %s", out)
	}
}

func TestRenderDocument_CodeHighlightUsesClasses(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "```go\nfunc main() {}\n```\n", resolveNone)))
	if !strings.Contains(out, `class="chroma"`) {
		t.Fatalf("expected chroma container, got: %s", out)
	}
	if strings.Contains(out, "style=") {
		t.Fatalf("highlighting must be class-based, found inline style: %s", out)
	}
}

func TestRenderDocument_BrokenWikilink(t *testing.T) {
	// resolveNone always returns false, so wikilinks render as broken spans.
	out := string(mustHTML(RenderDocument(context.Background(), "See [[NonExistentPage]] for details.\n", resolveNone)))
	if !strings.Contains(out, "wikilink-broken") {
		t.Fatalf("expected wikilink-broken span, got: %s", out)
	}
	if !strings.Contains(out, "NonExistentPage") {
		t.Fatalf("expected wikilink text in output, got: %s", out)
	}
}

func TestRenderDocument_ResolvedWikilink(t *testing.T) {
	resolve := func(target string) (href, title string, ok bool) {
		if target == "ExistingPage" {
			return "/wissen/doc-1", "Existing Page", true
		}
		return "", "", false
	}
	out := string(mustHTML(RenderDocument(context.Background(), "See [[ExistingPage]] for details.\n", resolve)))
	if !strings.Contains(out, `href="/wissen/doc-1"`) {
		t.Fatalf("expected resolved wikilink href, got: %s", out)
	}
	if strings.Contains(out, "wikilink-broken") {
		t.Fatalf("resolved wikilink should not be broken, got: %s", out)
	}
}

// --- Task L3-2: Mermaid as a set figure (AST transformer) --------------

func TestRenderDocument_MermaidBecomesFigure(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "# H\n\n```mermaid\ngraph TD; A-->B\n```\n"
	html, meta := RenderDocument(ctx, src, func(string) (string, string, bool) { return "", "", false })
	out := string(html)
	for _, want := range []string{`class="mermaid-figure"`, `<pre class="mermaid">`, "graph TD", "Abb. 1", "<details"} {
		if !strings.Contains(out, want) {
			t.Fatalf("mermaid figure misses %q:\n%s", want, out)
		}
	}
	if !meta.HasMermaid {
		t.Fatal("meta.HasMermaid must be true")
	}
	if strings.Contains(out, "<script") || strings.Contains(out, "hx-") {
		t.Fatalf("sanitizer leaked active markup:\n%s", out)
	}
}

func TestRenderDocument_TwoMermaidNumbered(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "```mermaid\nA\n```\n\ntext\n\n```mermaid\nB\n```\n"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if !strings.Contains(out, "Abb. 1") || !strings.Contains(out, "Abb. 2") {
		t.Fatalf("figures must number sequentially:\n%s", out)
	}
}

// TestRenderDocument_MermaidDoesNotBreakHighlighting is the regression guard
// (Codex #2): the mermaid figure is a dedicated AST node/renderer, never a
// FencedCodeBlock renderer — so normal code fences keep Chroma highlighting.
func TestRenderDocument_MermaidDoesNotBreakHighlighting(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "```go\nfmt.Println(\"x\")\n```\n\n```mermaid\nA-->B\n```\n"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if !strings.Contains(out, "class=\"mermaid-figure\"") {
		t.Fatal("mermaid block missing")
	}
	if !strings.Contains(out, "chroma") && !strings.Contains(out, "class=\"k") {
		t.Fatalf("go block lost chroma highlighting — mermaid renderer stole FencedCodeBlock:\n%s", out)
	}
}

func TestRenderDocument_RawHTMLNeutralized(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := `<div hx-get="/x" onclick="alert(1)">x</div>` + "\n\n<script>alert(1)</script>"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if strings.Contains(out, "hx-get") || strings.Contains(out, "onclick") || strings.Contains(out, "<script") {
		t.Fatalf("agent raw HTML must be neutralized:\n%s", out)
	}
}
