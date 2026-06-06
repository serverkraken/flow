package markdown_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/webui/markdown"
)

// goldenMD covers every element the notes view needs to render. The
// test asserts on a small set of stable HTML markers rather than the
// full byte-for-byte output so a goldmark patch upgrading whitespace
// doesn't flip the test red.
const goldenMD = `# Title One

Plain text with **bold** and *italic*. A [flow link](https://flow.example/notes) sits inline.

## Section A

Inline ` + "`code`" + ` is highlighted. Below is a block:

` + "```go" + `
package main

func main() {
    println("hello")
}
` + "```" + `

> Blockquote line.

- bullet one
- bullet two

1. ordered one
2. ordered two

### Subsection

| Col A | Col B |
|-------|-------|
| 1     | 2     |
`

func TestRenderer_AllElements(t *testing.T) {
	t.Parallel()
	r := markdown.New()
	out, err := r.Render([]byte(goldenMD))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := string(out)

	mustContain := []string{
		"<h1",                     // h1
		"Title One",               // h1 text
		"<h2",                     // h2
		"Section A",               //
		"<h3",                     // h3
		"<strong>bold</strong>",   // bold
		"<em>italic</em>",         // italic
		`href="https://flow.example/notes"`, // link href preserved
		"flow link",                         // link visible text
		"<code>code</code>",                 // inline code (default goldmark)
		"<pre",                              // code block
		"package",                           // code body content (chroma splits tokens)
		"main",                              //
		"hello",                             // code body string literal
		"<blockquote>",                      // blockquote
		"<ul>",                              // unordered list
		"<ol>",                              // ordered list
		"<table>",                           // table (GFM)
		"<th>Col A</th>",                    // table header
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

func TestRenderer_EscapesRawHTML(t *testing.T) {
	t.Parallel()
	r := markdown.New()
	out, err := r.Render([]byte(`A note: <script>alert(1)</script> end.`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := string(out)
	if strings.Contains(body, "<script>") {
		t.Errorf("raw <script> was passed through; must be omitted or escaped: %s", body)
	}
	if strings.Contains(body, "alert(1)") && strings.Contains(body, "<script>") {
		t.Errorf("script tag survived: %s", body)
	}
	// Goldmark's default policy is to drop literal HTML with a
	// "<!-- raw HTML omitted -->" marker — that's strictly safer than
	// escaping (it removes the structure entirely). Either policy is
	// acceptable; the invariant is "no live <script> tag in output".
}

func TestRenderer_Headings(t *testing.T) {
	t.Parallel()
	r := markdown.New()
	hs := r.Headings([]byte("# H1\n\n## Architektur\n\nbody\n\n### Detail\n\nmore\n\n## Offene Punkte\n"))
	if len(hs) != 3 {
		t.Fatalf("headings: got %d, want 3 (h2, h3, h2)", len(hs))
	}
	if hs[0].Level != 2 || hs[0].Text != "Architektur" {
		t.Errorf("first heading: %+v", hs[0])
	}
	if hs[1].Level != 3 || hs[1].Text != "Detail" {
		t.Errorf("second heading: %+v", hs[1])
	}
	if hs[2].Anchor != "offene-punkte" {
		t.Errorf("third heading anchor: got %q, want offene-punkte", hs[2].Anchor)
	}
}

func TestRenderer_EmptyInput(t *testing.T) {
	t.Parallel()
	r := markdown.New()
	out, err := r.Render(nil)
	if err != nil {
		t.Fatalf("Render(nil): %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected empty render for nil input; got %q", out)
	}
}
