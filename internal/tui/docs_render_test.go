package tui

import (
	"strings"
	"testing"
	"time"

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
