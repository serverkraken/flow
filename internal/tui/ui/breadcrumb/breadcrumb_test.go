package breadcrumb_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/breadcrumb"
)

func TestRender_emptyForSingleCrumb(t *testing.T) {
	if breadcrumb.Render([]string{"Home"}, theme.Default) != "" {
		t.Fatal("a single crumb (no drill-down) renders empty")
	}
	if breadcrumb.Render(nil, theme.Default) != "" {
		t.Fatal("nil renders empty")
	}
}

func TestRender_joinsCrumbs(t *testing.T) {
	got := breadcrumb.Render([]string{"Docs", "Note", "Backlink"}, theme.Default)
	for _, w := range []string{"Docs", "Note", "Backlink", "›"} {
		if !strings.Contains(got, w) {
			t.Fatalf("breadcrumb %q missing %q", got, w)
		}
	}
}
