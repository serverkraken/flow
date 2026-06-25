package webui

import (
	"strings"
	"testing"
)

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
	out := string(RenderDocument("| A | B |\n|---|---|\n| 1 | 2 |\n", resolveNone))
	if !strings.Contains(out, "<table") || !strings.Contains(out, "<td") {
		t.Fatalf("expected GFM table, got: %s", out)
	}
}

func TestRenderDocument_Tasklist(t *testing.T) {
	out := string(RenderDocument("- [x] done\n- [ ] todo\n", resolveNone))
	if !strings.Contains(out, `type="checkbox"`) {
		t.Fatalf("expected task checkboxes, got: %s", out)
	}
}

func TestRenderDocument_Strikethrough(t *testing.T) {
	out := string(RenderDocument("~~gone~~\n", resolveNone))
	if !strings.Contains(out, "<del>") {
		t.Fatalf("expected <del>, got: %s", out)
	}
}

func TestRenderDocument_Footnote(t *testing.T) {
	out := string(RenderDocument("Text[^1]\n\n[^1]: note\n", resolveNone))
	if !strings.Contains(out, `class="footnotes"`) && !strings.Contains(out, "footnote-ref") {
		t.Fatalf("expected footnote markup, got: %s", out)
	}
}

func TestRenderDocument_XSSStripped(t *testing.T) {
	out := string(RenderDocument("<script>alert(1)</script>\n\n[ok](javascript:alert(1))\n", resolveNone))
	if strings.Contains(out, "<script") || strings.Contains(out, "javascript:") {
		t.Fatalf("XSS not stripped: %s", out)
	}
}

func TestRenderDocument_CodeHighlightUsesClasses(t *testing.T) {
	out := string(RenderDocument("```go\nfunc main() {}\n```\n", resolveNone))
	if !strings.Contains(out, `class="chroma"`) {
		t.Fatalf("expected chroma container, got: %s", out)
	}
	if strings.Contains(out, "style=") {
		t.Fatalf("highlighting must be class-based, found inline style: %s", out)
	}
}
