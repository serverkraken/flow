package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func stripANSIForTest(s string) string { return ansi.Strip(s) }

// Regression: standalone `flow docs` must size the fullscreen overlay from the
// terminal WindowSizeMsg so it renders rich, scrollable markdown instead of
// falling back to the plain line renderer. (In `flow ui` the route's Frame
// bridge sizes it; standalone has only WindowSizeMsg.)
func TestDocs_WindowSizeMsgSizesViewerOverlay(t *testing.T) {
	doc := domain.Document{ID: "d1", Path: "n", Type: domain.DocFree, Title: "T", Body: "# Hello\n\nworld"}

	// Without a WindowSizeMsg the overlay has no size → fallback (empty
	// overlayView): the pre-fix standalone behaviour Soenne hit.
	nosize, _ := NewDocs(nil, nil, nil, theme.Default, "t").Update(docViewMsg{doc: doc})
	if ov := nosize.(DocsModel).overlayView(); ov != "" {
		t.Fatalf("no WindowSizeMsg: overlay should be unsized (fallback), got:\n%s", ov)
	}

	// After a size, opening the doc renders the fullscreen markdown overlay.
	sized, _ := NewDocs(nil, nil, nil, theme.Default, "t").Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	shown, _ := sized.(DocsModel).Update(docViewMsg{doc: doc})
	ov := shown.(DocsModel).overlayView()
	if ov == "" {
		t.Fatal("after WindowSizeMsg + docViewMsg the overlay should render (non-empty)")
	}
	if plain := stripANSIForTest(ov); !strings.Contains(plain, "Hello") || !strings.Contains(plain, "world") {
		t.Fatalf("overlay should render the markdown body, got:\n%s", plain)
	}
}

// Regression: search results render in the SAME kompendium list-row style as the
// normal docs view (no surrounding box), with single-line, markdown-stripped
// snippets (no raw `###`, no ragged indentation) as the row excerpt.
func TestDocs_SearchResultsListStyleAndCleaned(t *testing.T) {
	m := NewDocs(nil, nil, nil, theme.Default, "t")
	m.width = 80
	m.mode = modeSearch
	m.searching = true
	m.searchQuery = "arnold"
	m.searchHits = []domain.SearchHit{{
		Document: domain.Document{Title: "2026-04-28", Path: "daily/2026-06-19"},
		Snippet:  "## Tickets\n\n### DPH-3631\n\n    Arnold approved the commit",
	}}

	out := stripANSIForTest(m.View().Content)
	if strings.Contains(out, "╭") || strings.Contains(out, "╰") {
		t.Fatalf("search results must NOT be boxed — use the plain list-row look:\n%s", out)
	}
	if strings.Contains(out, "###") {
		t.Fatalf("snippet should drop raw markdown heading markers:\n%s", out)
	}
	if !strings.Contains(out, "Tickets") || !strings.Contains(out, "Arnold") {
		t.Fatalf("snippet text should survive cleaning:\n%s", out)
	}
}
