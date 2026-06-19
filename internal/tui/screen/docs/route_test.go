package docs_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/docs"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestDocsRoute_titleAndRenders(t *testing.T) {
	// client nil + nil editor/opener: DocsModel renders its list chrome without
	// touching the network until Init's cmd runs (which we don't drain here).
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	if r.Title() != "Docs" {
		t.Fatalf("title = %q, want Docs", r.Title())
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "docs") { // DocsModel renders a "flow · docs" header
		t.Fatalf("docs body should contain the docs header:\n%s", body)
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("docs route should expose key hints")
	}
}

func TestDocsRoute_updateReturnsRoute(t *testing.T) {
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	r2, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r2 == nil {
		t.Fatal("Update must return a Route")
	}
}
