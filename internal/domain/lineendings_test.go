package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNormalizeLineEndings pins a Bestandsfehler that hid for weeks: browsers
// normalise a <textarea> to CRLF on form submit (HTML spec), the handlers
// stored it verbatim, and fencedBlock only knows "---\n". Every card saved in
// the web editor silently lost its frontmatter — 69 of 512 in the dev DB when
// this was found. One normalisation at the domain edge fixes web, REST and MCP
// alike; the editor is not the culprit, it merely made the damage visible.
func TestNormalizeLineEndings(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"CRLF → LF", "---\r\ntype: spec\r\n---\r\n# T\r\n", "---\ntype: spec\n---\n# T\n"},
		{"lone CR → LF", "a\rb", "a\nb"},
		{"LF untouched", "a\nb\n", "a\nb\n"},
		{"mixed", "a\r\nb\nc\rd", "a\nb\nc\nd"},
		{"empty", "", ""},
	} {
		if got := domain.NormalizeLineEndings(c.in); got != c.want {
			t.Errorf("%s: NormalizeLineEndings(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestParseFrontmatter_SurvivesCRLFAfterNormalize is the end-to-end claim:
// a CRLF body, once normalised, yields its frontmatter again.
func TestParseFrontmatter_SurvivesCRLFAfterNormalize(t *testing.T) {
	crlf := "---\r\ntags: [editor]\r\n---\r\n# T\r\n"
	if _, start := domain.ParseFrontmatter(crlf); start != 0 {
		t.Fatalf("precondition: raw CRLF must NOT parse (that is the bug), got start=%d", start)
	}
	tags, start := domain.ParseFrontmatter(domain.NormalizeLineEndings(crlf))
	if start == 0 || len(tags) != 1 || tags[0] != "editor" {
		t.Errorf("after normalise: start=%d tags=%v, want frontmatter recognised with tag 'editor'", start, tags)
	}
}
