package webui

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestGroupDocsByCategory(t *testing.T) {
	docs := []domain.Document{
		{ID: "1", Type: domain.DocDaily, Title: "Mon"},
		{ID: "2", Type: domain.DocProject, Title: "Note", NodeID: strptr("p1")},
		{ID: "3", Type: domain.DocFree, Title: "Urlaub"},
		{ID: "4", Type: domain.DocMemory, Title: "Mem"},
	}
	names := map[string]string{"p1": "Alpha"}
	colors := map[string]string{"p1": "blue"}

	vm := groupDocsByCategory(docs, names, colors, nil)

	if len(vm.Daily) != 1 || len(vm.Free) != 1 || len(vm.System) != 1 {
		t.Fatalf("category split wrong: %+v", vm)
	}
	if len(vm.Notes) != 1 || vm.Notes[0].Name != "Alpha" || len(vm.Notes[0].Docs) != 1 {
		t.Fatalf("project grouping wrong: %+v", vm.Notes)
	}
}

func TestWissenCategoryFromSlug(t *testing.T) {
	tests := map[string]struct {
		wantID string
		ok     bool
	}{
		"daily":    {"daily", true},
		"projekte": {"projekte", true},
		"frei":     {"frei", true},
		"system":   {"system", true},
		"bogus":    {"", false},
	}
	for slug, tt := range tests {
		got, ok := WissenCategoryFromSlug(slug)
		if ok != tt.ok || got.ID != tt.wantID {
			t.Fatalf("WissenCategoryFromSlug(%q) = (%q,%v), want (%q,%v)", slug, got.ID, ok, tt.wantID, tt.ok)
		}
	}
}

func TestDocPreviewTextStripsMarkdownAndLimitsLines(t *testing.T) {
	body := "---\ntags: [x]\n---\n# Heading\n\n- first [[daily/2026-06-25|Daily Link]]\n<script>alert(1)</script>\n```terraform\nresource \"x\" \"y\" {}\n```\nplain **bold** text\nsixth line\n"
	got := DocPreviewText(body, 5)
	for _, bad := range []string{"---", "tags:", "<script>", "```", "**", "[[", "]]"} {
		if strings.Contains(got, bad) {
			t.Fatalf("preview leaked %q: %q", bad, got)
		}
	}
	lines := strings.Split(got, "\n")
	if len(lines) > 5 {
		t.Fatalf("preview lines=%d, want <=5: %q", len(lines), got)
	}
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "Daily Link") {
		t.Fatalf("preview missing readable content: %q", got)
	}
}

func TestBuildWissenOverviewCountsAndLatest(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocDaily, Title: "Old daily", UpdatedAt: now.Add(-time.Hour)},
		{ID: "d2", Type: domain.DocDaily, Title: "New daily", UpdatedAt: now},
		{ID: "p1", Type: domain.DocProject, Title: "Project", UpdatedAt: now},
		{ID: "m1", Type: domain.DocMemory, Title: "Memory", UpdatedAt: now},
	}
	vm := BuildWissenOverview(docs, nil, nil, nil)
	daily := vm.Categories[0]
	if daily.Count != 2 || len(daily.Latest) != 2 || daily.Latest[0].Title != "New daily" {
		t.Fatalf("daily overview = %+v", daily)
	}
	system := vm.Categories[3]
	if system.Count != 1 || system.Latest[0].Title != "Memory" {
		t.Fatalf("system overview = %+v", system)
	}
}

func strptr(s string) *string { return &s }

func TestWissenTabsAndEmpty(t *testing.T) {
	empty := WissenVM{}
	if !WissenEmpty(empty) {
		t.Error("WissenEmpty on zero vm should be true")
	}
	tabs := WissenTabs(empty)
	if len(tabs) != 4 {
		t.Fatalf("expected 4 tabs, got %d", len(tabs))
	}

	nonEmpty := WissenVM{Daily: []DocRow{{ID: "d1"}}}
	if WissenEmpty(nonEmpty) {
		t.Error("WissenEmpty on non-zero vm should be false")
	}
	tabs2 := WissenTabs(nonEmpty)
	if tabs2[0].Count != 1 {
		t.Errorf("daily tab count = %d, want 1", tabs2[0].Count)
	}
}

func TestWissenResetHref(t *testing.T) {
	if wissenResetHref(WissenVM{}) != "/wissen" {
		t.Error("default reset href should be /wissen")
	}
	if wissenResetHref(WissenVM{ResetHref: "/wissen?tag=x"}) != "/wissen?tag=x" {
		t.Error("custom reset href not returned")
	}
}

func TestProjectDocCountAndSwatchStyle(t *testing.T) {
	groups := []ProjectGroup{
		{Name: "A", Docs: []DocRow{{ID: "1"}, {ID: "2"}}},
		{Name: "B", Docs: []DocRow{{ID: "3"}}},
	}
	if n := projectDocCount(groups); n != 3 {
		t.Errorf("projectDocCount = %d, want 3", n)
	}

	if s := swatchStyle(""); s != "" {
		t.Errorf("swatchStyle('') = %q, want ''", s)
	}
	got := swatchStyle("#aabbcc")
	if got != "--swatch: #aabbcc" {
		t.Errorf("swatchStyle('#aabbcc') = %q, want '--swatch: #aabbcc'", got)
	}
}
