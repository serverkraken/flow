package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func strptr(s string) *string { return &s }

func TestApplyProjectFilter_Inclusive(t *testing.T) {
	t.Parallel()
	docs := []domain.Document{
		{Type: domain.DocDaily},                                  // nil ProjectID
		{Type: domain.DocProject, ProjectID: strptr("p1")},
		{Type: domain.DocProject, ProjectID: strptr("p2")},
		{Type: domain.DocFree},                                   // nil ProjectID
	}
	got := applyProjectFilter(docs, "p1")
	if len(got) != 3 { // daily + p1 + free
		t.Fatalf("filtered len = %d, want 3 (daily+p1+free)", len(got))
	}
	if all := applyProjectFilter(docs, ""); len(all) != 4 {
		t.Fatalf("empty filter len = %d, want 4", len(all))
	}
}

func TestDateCell_DailyUsesDate_ElseUpdatedAt(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	daily := domain.Document{Type: domain.DocDaily, Date: &d, UpdatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	if got := dateCell(daily); got != "2026-05-18" {
		t.Errorf("daily dateCell = %q, want 2026-05-18", got)
	}
	free := domain.Document{Type: domain.DocFree, UpdatedAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}
	if got := dateCell(free); got != "2026-06-10" {
		t.Errorf("free dateCell = %q, want 2026-06-10", got)
	}
}

func TestProjRowLabel(t *testing.T) {
	t.Parallel()
	byID := map[string]domain.Project{"p1": {ID: "p1", Slug: "serverkraken/flow"}}
	proj := domain.Document{Type: domain.DocProject, ProjectID: strptr("p1"), Title: "demo", Path: "x/demo"}
	if got := projRowLabel(proj, byID); got != "serverkraken/flow · demo" {
		t.Errorf("projRowLabel = %q, want 'serverkraken/flow · demo'", got)
	}
	free := domain.Document{Type: domain.DocFree, Path: "notes/foo"}
	if got := projRowLabel(free, byID); got != "notes/foo" {
		t.Errorf("free projRowLabel = %q, want 'notes/foo'", got)
	}
}

func TestDocExcerpt_WrapAndCap(t *testing.T) {
	t.Parallel()
	lines := docExcerpt("alpha beta gamma delta epsilon zeta eta", 11, 2)
	if len(lines) != 2 {
		t.Fatalf("excerpt lines = %d, want 2", len(lines))
	}
	if last := lines[1]; !strings.HasSuffix(last, "…") {
		t.Errorf("truncated line %q does not end with …", last)
	}
	if w := lipgloss.Width(lines[1]); w > 11 {
		t.Errorf("truncated line width %d > max 11", w)
	}
	if got := docCounts([]domain.Document{{Type: domain.DocDaily}, {Type: domain.DocDaily}, {Type: domain.DocFree}}); got[domain.DocDaily] != 2 || got[domain.DocFree] != 1 {
		t.Errorf("docCounts = %+v, want daily:2 free:1", got)
	}
}

func TestUpdate_ProjectsLoaded_BuildsIndex(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	nm, _ := m.Update(projectsLoadedMsg{projects: []domain.Project{
		{ID: "p1", Slug: "serverkraken/flow"},
		{ID: "p2", Slug: "other/repo"},
	}})
	dm := nm.(DocsModel)
	if len(dm.projByID) != 2 || dm.projByID["p1"].Slug != "serverkraken/flow" {
		t.Fatalf("projByID = %+v, want 2 entries indexed by id", dm.projByID)
	}
}

func TestRenderList_KompendiumLook(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.width = 80
	m.height = 40
	m.docs = []domain.Document{
		{Type: domain.DocDaily, Path: "daily/2026-05-18", Date: &d, Body: "First on-call schedule note"},
		{Type: domain.DocFree, Path: "notes/foo", UpdatedAt: d},
	}
	out := m.View().Content
	for _, want := range []string{"kompendium", "Notizen", "TÄGL.", "FREI", "daily/2026-05-18", "2026-05-18", "1/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderList missing %q in:\n%s", want, out)
		}
	}
}

// TestFilteredCursorDesync verifies that with an active project filter the
// cursor (m.sel) maps to the VISIBLE set, not the unfiltered m.docs slice.
// Before the fix, j/k clamped against len(m.docs) and enter/e/d indexed
// m.docs[m.sel], so a hidden doc sorted before visible ones would cause the
// wrong document to be opened or deleted.
func TestFilteredCursorDesync(t *testing.T) {
	t.Parallel()

	// docs[0] belongs to p2 (hidden when filter = "p1")
	// docs[1] and docs[2] are free (visible for any project filter)
	docs := []domain.Document{
		{ID: "hidden", Type: domain.DocProject, ProjectID: strptr("p2"), Path: "x/hidden"},
		{ID: "free-a", Type: domain.DocFree, Path: "free-a"},
		{ID: "free-c", Type: domain.DocFree, Path: "free-c"},
	}

	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.docs = docs
	m.projFilter = "p1" // only free docs are visible; the p2 doc is hidden

	vis := m.visibleDocs()
	if len(vis) != 2 {
		t.Fatalf("visibleDocs len = %d, want 2", len(vis))
	}
	if vis[0].Path != "free-a" {
		t.Errorf("visibleDocs[0].Path = %q, want free-a", vis[0].Path)
	}
	if vis[1].Path != "free-c" {
		t.Errorf("visibleDocs[1].Path = %q, want free-c", vis[1].Path)
	}

	// sel=0 must map to free-a (not the hidden p2 doc at docs[0]).
	// Simulate what the enter handler now does: index vis[m.sel].
	if vis[m.sel].ID != "free-a" {
		t.Errorf("vis[sel=%d].ID = %q, want free-a (enter would open wrong doc)", m.sel, vis[m.sel].ID)
	}

	// Pressing j must not advance sel past the VISIBLE set boundary (len 2, not 3).
	nm, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = nm.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("after j: sel = %d, want 1", m.sel)
	}
	nm, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	m = nm.(DocsModel)
	if m.sel != 1 {
		t.Errorf("after second j at boundary: sel = %d, want 1 (clamped to visible len-1)", m.sel)
	}
}

func TestProjectFilter_OpenSelectClear(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.projects = []domain.Project{{ID: "p1", Slug: "serverkraken/flow"}}
	m.projByID = map[string]domain.Project{"p1": m.projects[0]}

	// open picker with "p"
	nm, _ := m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	if m.mode != modeProjectFilter {
		t.Fatalf("after p: mode = %v, want modeProjectFilter", m.mode)
	}
	// move to the project (index 1) and select
	m.projCursor = 1
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "p1" {
		t.Fatalf("projFilter = %q, want p1", m.projFilter)
	}
	if m.mode != modeList {
		t.Fatalf("after select: mode = %v, want modeList", m.mode)
	}
	// re-open, choose index 0 ("Alle") to clear
	nm, _ = m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	m.projCursor = 0
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "" {
		t.Fatalf("after Alle: projFilter = %q, want empty", m.projFilter)
	}
}
