package markdown_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown"
)

func TestRender_BasicMarkdown(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("# Hello\n\nWorld", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("output missing heading text: %q", out)
	}
	if !strings.Contains(out, "World") {
		t.Errorf("output missing body text: %q", out)
	}
}

func TestRender_EmptySource(t *testing.T) {
	t.Parallel()
	out, err := markdown.Render("", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = out
}

func TestRender_CodeBlock(t *testing.T) {
	t.Parallel()
	src := "```go\nfmt.Println(\"hi\")\n```"
	out, err := markdown.Render(src, 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Println") {
		t.Errorf("output missing code content: %q", out)
	}
}
