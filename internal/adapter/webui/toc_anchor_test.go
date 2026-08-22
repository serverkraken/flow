package webui

import (
	"os"
	"strings"
	"testing"
)

// TestTocJS_StripsHeadingAnchor pins that the client-side table of contents
// takes a heading's text WITHOUT its hover ¶ anchor (head-anchor, appended
// server-side as the heading's last child) — the anchor showed up as a
// trailing "¶" on every entry (Soenne, 22.08.).
func TestTocJS_StripsHeadingAnchor(t *testing.T) {
	b, err := os.ReadFile("static/toc.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, ".head-anchor") {
		t.Fatalf("toc.js must strip .head-anchor before reading heading text:\n%s", src)
	}
	if strings.Contains(src, "link.textContent = heading.textContent") {
		t.Fatalf("toc.js still copies the raw heading text (includes the ¶ anchor)")
	}
}
