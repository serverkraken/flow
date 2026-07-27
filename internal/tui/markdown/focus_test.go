package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// stubFocusResolver resolves every target as valid (distinct from
// stubWikiResolver in wikilink_test.go which is map-based).
type stubFocusResolver struct{}

func (stubFocusResolver) Resolve(target string) (string, string, bool) {
	return "flow://docs/" + target, target, true
}

// TestWithFocusedWikilink_HighlightsNthValid focuses the 2nd valid wikilink
// and asserts the rendered output differs from the no-focus render.
func TestWithFocusedWikilink_HighlightsNthValid(t *testing.T) {
	src := "see [[alpha]] and [[beta]] today"
	none, err := Render(src, 80, WithWikilinks(stubFocusResolver{}), WithFocusedWikilink(-1))
	if err != nil {
		t.Fatal(err)
	}
	focused, err := Render(src, 80, WithWikilinks(stubFocusResolver{}), WithFocusedWikilink(1))
	if err != nil {
		t.Fatal(err)
	}
	if none == focused {
		t.Fatal("focusing a wikilink should change the rendered output")
	}
	// Both still contain both link displays (ANSI-stripped).
	plain := ansi.Strip(focused)
	if !strings.Contains(plain, "alpha") || !strings.Contains(plain, "beta") {
		t.Fatalf("both wikilinks should still render:\n%s", plain)
	}
}
