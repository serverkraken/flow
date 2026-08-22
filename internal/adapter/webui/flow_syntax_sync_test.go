package webui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestFlowSyntax_CalloutGlyphsMatchServer keeps the editor's callout table
// (web/editor/flow-syntax.mjs, CALLOUT_GLYPH) identical to the server's
// (markdown_callout.go, calloutGlyph): same kinds, same glyphs — a callout
// looks the same while writing as after saving, and an unknown kind stays a
// quote on both sides.
func TestFlowSyntax_CalloutGlyphsMatchServer(t *testing.T) {
	b, err := os.ReadFile("../../../web/editor/flow-syntax.mjs")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	m := regexp.MustCompile(`CALLOUT_GLYPH = \{([^}]*)\}`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal("flow-syntax.mjs: CALLOUT_GLYPH table not found")
	}
	got := map[string]string{}
	for _, kv := range regexp.MustCompile(`(\w+):\s*'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
		got[kv[1]] = kv[2]
	}
	if len(got) != len(calloutGlyph) {
		t.Errorf("editor knows %d callout kinds, server %d", len(got), len(calloutGlyph))
	}
	for kind, glyph := range calloutGlyph {
		if got[kind] != glyph {
			t.Errorf("kind %q: editor glyph %q, server glyph %q", kind, got[kind], glyph)
		}
		if !calloutKinds[kind] {
			t.Errorf("server glyph table has kind %q that calloutKinds does not accept", kind)
		}
	}
}

// TestEditorBundle_WiresReadingViewFigures pins that the editor source renders
// ```mermaid as the reading view's figure (mermaid-figure via the
// window.flowMermaid bridge) and embeds as figure.figure — not as raw text.
func TestEditorBundle_WiresReadingViewFigures(t *testing.T) {
	for file, wants := range map[string][]string{
		"../../../web/editor/editor.mjs":     {"renderPreview: mermaidPreview", "window.flowMermaid", "mermaid-figure"},
		"../../../web/editor/flow-views.mjs": {"/ui/editor/aufloesen", "wikilink-broken", "className = 'figure'", "callout-title"},
		"static/js/mermaid-init.js":          {"window.flowMermaid"},
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s: missing %q", file, want)
			}
		}
	}
}
