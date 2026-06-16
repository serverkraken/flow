package keyhint_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

func TestRender_includesKeysAndDescs(t *testing.T) {
	got := keyhint.Render([]keyhint.Hint{{Key: "tab", Desc: "Tab wechseln"}, {Key: ":", Desc: "Palette"}}, theme.Default)
	for _, want := range []string{"tab", "Tab wechseln", ":", "Palette"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hints %q missing %q", got, want)
		}
	}
}

func TestRender_capsAtFour(t *testing.T) {
	hints := []keyhint.Hint{{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}, {Key: "e"}, {Key: "f"}}
	got := keyhint.Render(hints, theme.Default)
	if strings.Contains(got, "f") || strings.Contains(got, "e") {
		t.Fatalf("expected only first 4 hints rendered, got %q", got)
	}
}

func TestRender_emptyIsEmpty(t *testing.T) {
	if keyhint.Render(nil, theme.Default) != "" {
		t.Fatal("nil hints should render empty string")
	}
}
