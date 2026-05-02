package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// stubWikiResolver is a tiny test resolver: returns ok=true for every
// target listed in known. Title falls back to the target itself.
type stubWikiResolver struct {
	known map[string]string // target → uri
}

func (r stubWikiResolver) Resolve(target string) (uri, title string, ok bool) {
	if uri, ok := r.known[target]; ok {
		return uri, target, true
	}
	return "", "", false
}

// TestRender_Wikilink_ValidGetsOSC8: a wikilink whose target the
// resolver knows comes out wrapped in an OSC 8 sequence pointing at
// the resolver's URI, with the valid-glyph (⇲) prefix.
func TestRender_Wikilink_ValidGetsOSC8(t *testing.T) {
	t.Parallel()
	res := stubWikiResolver{known: map[string]string{
		"projects/foo": "kompendium://note/projects/foo",
	}}
	out, err := Render("siehe [[projects/foo]] dort", 60, WithWikilinks(res))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "⇲ projects/foo") {
		t.Errorf("valid wikilink missing glyph + display: %q", plain)
	}
	if !strings.Contains(out, "kompendium://note/projects/foo\x07") {
		t.Errorf("valid wikilink missing OSC 8 destination: %q", out)
	}
}

// TestRender_Wikilink_BrokenGetsRedMarker: an unknown target wears
// the broken style + ⌧ glyph and is NOT wrapped in OSC 8 (a dead
// link shouldn't pretend to be clickable).
func TestRender_Wikilink_BrokenGetsRedMarker(t *testing.T) {
	t.Parallel()
	res := stubWikiResolver{known: map[string]string{}}
	out, err := Render("siehe [[unknown]] hier", 60, WithWikilinks(res))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "⌧ unknown") {
		t.Errorf("broken wikilink missing marker: %q", plain)
	}
	if strings.Contains(out, "\x1b]8;") && strings.Contains(out, "unknown") {
		// The id stamp + URI must NOT contain the broken target.
		if strings.Contains(out, ";unknown\x07") {
			t.Errorf("broken wikilink should not carry OSC 8 destination: %q", out)
		}
	}
}

// TestRender_Wikilink_DisplayOverridesTarget: `[[id|display]]` uses
// the display half of the pipe split as the visible text, not the
// target id.
func TestRender_Wikilink_DisplayOverridesTarget(t *testing.T) {
	t.Parallel()
	res := stubWikiResolver{known: map[string]string{
		"projects/foo": "kompendium://note/projects/foo",
	}}
	out, err := Render("[[projects/foo|Mein Projekt]]", 60, WithWikilinks(res))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Mein Projekt") {
		t.Errorf("display override missing: %q", plain)
	}
	if strings.Contains(plain, "projects/foo") {
		t.Errorf("target id leaked into visible output: %q", plain)
	}
}

// TestRender_Wikilink_NoResolverFallsBackToBroken: without a
// WithWikilinks option, every wikilink is treated as broken so the
// renderer surfaces them clearly instead of silently dropping the
// brackets.
func TestRender_Wikilink_NoResolverFallsBackToBroken(t *testing.T) {
	t.Parallel()
	out, err := Render("[[anything]]", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "⌧ anything") {
		t.Errorf("no-resolver fallback should render as broken: %q", plain)
	}
}

// TestRender_Wikilink_OSC8IDStampPresent: the OSC 8 sequence
// includes an `id=` stamp so terminals can join multi-line wraps
// into one click target.
func TestRender_Wikilink_OSC8IDStampPresent(t *testing.T) {
	t.Parallel()
	res := stubWikiResolver{known: map[string]string{"x": "kompendium://note/x"}}
	out, err := Render("[[x]]", 60, WithWikilinks(res))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b]8;id=") {
		t.Errorf("OSC 8 missing id= stamp: %q", out)
	}
}
