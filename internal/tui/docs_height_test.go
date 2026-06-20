package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// In standalone `flow docs` the whole DocsModel View must fit within the
// terminal height: a paginated list page plus the countbar and the keyhint
// footer ("bottombar") must never exceed m.height, or the footer is pushed
// below the fold and only appears after scrolling. Each doc row is up to 4
// lines (title + up to 2 excerpt lines + blank), so the per-page count must be
// derived from that worst case — not 3 lines/row.
func TestDocsList_fitsWindowHeight(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	docs := make([]domain.Document, 30)
	for i := range docs {
		docs[i] = domain.Document{
			ID:    fmt.Sprintf("d%d", i),
			Type:  domain.DocFree,
			Title: fmt.Sprintf("Note %d", i),
			// Long body → docExcerpt yields the full 2 lines → 4-line rows.
			Body: strings.Repeat("lorem ipsum dolor sit amet consectetur adipiscing elit ", 6),
		}
	}
	m.docs = docs

	for _, h := range []int{18, 24, 40, 60} {
		mm := m.SetViewport(80, h)
		assertFits(t, mm, h, "default")

		// A transient status line (set after view/edit/delete) must also not
		// push the footer below the fold.
		withStatus := mm
		withStatus.status = "gespeichert"
		assertFits(t, withStatus, h, "with-status")
	}
}

// rowOf returns the 0-based index of the first line containing sub, or -1.
func rowOf(content, sub string) int {
	for i, ln := range strings.Split(content, "\n") {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

// When scrolling between pages of differing content height, the bottom bar
// (pager line + keyhint footer) must stay pinned to the same rows — it must not
// move as the user pages through the list.
func TestDocsList_bottomBarStableAcrossScroll(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	docs := make([]domain.Document, 30)
	for i := range docs {
		body := strings.Repeat("lorem ipsum dolor sit amet ", 6)
		if i%2 == 0 {
			body = "" // half the rows are short → variable page heights
		}
		docs[i] = domain.Document{ID: fmt.Sprintf("d%d", i), Type: domain.DocFree, Title: fmt.Sprintf("Note %d", i), Body: body}
	}
	m.docs = docs
	const h = 30
	m = m.SetViewport(80, h)

	m.sel = 0
	firstFooter := rowOf(m.View().Content, "q quit")
	firstPager := rowOf(m.View().Content, "/30")

	m.sel = len(docs) - 1 // jump to the last page (fewer, possibly shorter rows)
	lastFooter := rowOf(m.View().Content, "q quit")
	lastPager := rowOf(m.View().Content, "/30")

	if firstFooter != lastFooter {
		t.Errorf("footer row moved on scroll: page0=%d lastPage=%d", firstFooter, lastFooter)
	}
	if firstPager != lastPager {
		t.Errorf("pager row moved on scroll: page0=%d lastPage=%d", firstPager, lastPager)
	}
}

// Search results must also fit the window and keep their footer ("bottombar")
// pinned/stable as the user moves the selection through many hits — the same
// windowing the normal list uses.
func TestDocsSearch_fitsWindowAndStableFooter(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.mode = modeSearch
	m.searching = true
	m.searchQuery = "lorem"
	hits := make([]domain.SearchHit, 25)
	for i := range hits {
		hits[i] = domain.SearchHit{
			Document: domain.Document{ID: fmt.Sprintf("d%d", i), Type: domain.DocFree, Title: fmt.Sprintf("Note %d", i)},
			Snippet:  strings.Repeat("lorem ipsum dolor sit amet ", 6),
		}
	}
	m.searchHits = hits
	const h = 28
	m = m.SetViewport(80, h)

	m.searchSel = 0
	first := m.View().Content
	if got := strings.Count(first, "\n") + 1; got > h {
		t.Errorf("search view is %d lines (>%d) — results overflow the window", got, h)
	}
	if !strings.Contains(first, "esc abbrechen") {
		t.Errorf("search footer missing from view at top of results")
	}
	footerRow0 := rowOf(first, "esc abbrechen")

	m.searchSel = len(hits) - 1 // scroll to last result
	last := m.View().Content
	if got := strings.Count(last, "\n") + 1; got > h {
		t.Errorf("search view is %d lines (>%d) at last result — overflow", got, h)
	}
	if r := rowOf(last, "esc abbrechen"); r != footerRow0 {
		t.Errorf("search footer moved on scroll: top=%d last=%d", footerRow0, r)
	}
}

func assertFits(t *testing.T, m DocsModel, h int, label string) {
	t.Helper()
	v := m.View()
	if got := strings.Count(v.Content, "\n") + 1; got > h {
		t.Errorf("window 80x%d (%s): view is %d lines (>%d) — footer pushed below the fold", h, label, got, h)
	}
	if !strings.Contains(v.Content, "q quit") {
		t.Errorf("window 80x%d (%s): footer keyhint missing from view", h, label)
	}
}
