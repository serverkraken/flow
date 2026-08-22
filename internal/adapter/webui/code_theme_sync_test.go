package webui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestCodeTheme_MatchesChroma keeps the editor's CodeMirror colours
// (web/editor/code-theme.mjs) identical to the reading view's chroma style
// (chromacss.go, "flow-karteikasten"): a code block must look the same while
// writing as after saving. The hex values are the contract.
func TestCodeTheme_MatchesChroma(t *testing.T) {
	b, err := os.ReadFile("../../../web/editor/code-theme.mjs")
	if err != nil {
		t.Fatal(err)
	}
	theme := string(b)
	g, err := os.ReadFile("chromacss.go")
	if err != nil {
		t.Fatal(err)
	}
	chroma := string(g)
	pick := func(entry string) string {
		m := regexp.MustCompile(`chroma\.` + entry + `:\s*"(#[0-9A-Fa-f]{6})`).FindStringSubmatch(chroma)
		if m == nil {
			t.Fatalf("chromacss.go: no colour for %s", entry)
		}
		return m[1]
	}
	for key, entry := range map[string]string{
		"text": "Text", "keyword": "Keyword", "number": "LiteralNumber",
		"string": "LiteralString", "comment": "Comment", "deleted": "GenericDeleted",
	} {
		want := key + ": '" + pick(entry) + "'"
		if !strings.Contains(theme, want) {
			t.Errorf("code-theme.mjs: %s must carry chroma's %s colour (%s)", key, entry, want)
		}
	}
}
