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
	for _, want := range []string{`data-document-prose class="min-w-0`, `class="min-w-0 space-y-4`} {
		if !strings.Contains(out, want) {
			t.Fatalf("DocumentFragment missing layout guard %q in %.600s", want, out)
		}
	}
}

func TestDocumentFragmentPlacesMobileTocBeforeMarkdownContent(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "Wide document", KindLabel: "Frei", KindGlyph: "F", KindTone: "free"}
	var buf bytes.Buffer
	if err := DocumentFragment(vm).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-mobile-toc`,
		`class="mb-6 lg:hidden"`,
		`data-document-prose`,
		`data-desktop-toc`,
		`class="hidden lg:block"`,
		`id="toc-mobile"`,
		`id="toc-desktop"`,
		`data-document-backlinks`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("DocumentFragment missing mobile toc layout marker %q in %.800s", want, out)
		}
	}
	if got := strings.Count(out, `data-toc-nav`); got != 2 {
		t.Fatalf("expected 2 toc navs, got %d in %.800s", got, out)
	}

	mobileToc := strings.Index(out, `data-mobile-toc`)
	prose := strings.Index(out, `data-document-prose`)
	desktopToc := strings.Index(out, `data-desktop-toc`)
	backlinks := strings.Index(out, `data-document-backlinks`)
	if mobileToc >= prose || prose >= desktopToc || desktopToc >= backlinks {
		t.Fatalf("expected mobile toc before prose and desktop rail after prose; indexes mobile=%d prose=%d desktop=%d backlinks=%d", mobileToc, prose, desktopToc, backlinks)
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
