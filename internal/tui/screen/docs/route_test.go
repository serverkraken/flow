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

// TestDocsRoute_implementsFullScreenerListFalse asserts the docs route satisfies
// shell.FullScreener and reports false while in the document list (the true case
// needs an unexported docViewMsg, covered in the package tui test + done-gate).
func TestDocsRoute_implementsFullScreenerListFalse(t *testing.T) {
	r := docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	fs, ok := interface{}(r).(shell.FullScreener)
	if !ok {
		t.Fatal("docs.Route must implement shell.FullScreener")
	}
	if fs.FullScreen() {
		t.Fatal("list mode: FullScreen() must be false")
	}
}

// TestDocsRoute_capturesInputInSubmode guards the bug where the shell ate the
// New-Document form's Tab/Esc keys: the adapter must implement
// shell.InputCapturer and report capture once the docs screen leaves list mode.
func TestDocsRoute_capturesInputInSubmode(t *testing.T) {
	r := docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	ic, ok := interface{}(r).(shell.InputCapturer)
	if !ok {
		t.Fatal("docs.Route must implement shell.InputCapturer")
	}
	if ic.CapturesInput() {
		t.Fatal("list mode: route must not capture (host nav must work)")
	}
	// 'n' opens the create form; the adapter must now report capture so the
	// shell forwards Tab/Esc to the form instead of switching tabs.
	r.Update(tea.KeyPressMsg{Text: "n"})
	if !ic.CapturesInput() {
		t.Fatal("create mode: route must report CapturesInput()==true")
	}
}
