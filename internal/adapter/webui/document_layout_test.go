package webui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestDocumentFragmentConstrainsMarkdownColumn(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "Wide document", KindLabel: "Frei", KindGlyph: "F", KindTone: "free"}
	var buf bytes.Buffer
	if err := DocumentFragment(vm).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`<section class="min-w-0`, `class="min-w-0 space-y-4`} {
		if !strings.Contains(out, want) {
			t.Fatalf("DocumentFragment missing layout guard %q in %.600s", want, out)
		}
	}
}

func TestMarkdownProseCSSGuardsWideContent(t *testing.T) {
	css, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := string(css)
	for _, want := range []string{
		"overflow-wrap: anywhere;",
		".prose a { @apply text-accent underline underline-offset-2 break-words; }",
		".prose code {",
		"word-break: break-word;",
		".prose pre {",
		"max-width: 100%;",
		"overflow-x: auto;",
		".prose table {",
		"display: block;",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("web/tailwind.css missing markdown overflow guard %q", want)
		}
	}
}
