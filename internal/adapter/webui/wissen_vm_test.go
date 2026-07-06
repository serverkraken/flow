package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWissenShelvesOrderAndTypeKeys(t *testing.T) {
	shelves := WissenShelves()
	wantKeys := []string{"project", "plan", "spec", "memory", "daily", "context", "free"}
	if len(shelves) != len(wantKeys) {
		t.Fatalf("WissenShelves() = %d shelves, want %d", len(shelves), len(wantKeys))
	}
	for i, want := range wantKeys {
		if shelves[i].TypeKey != want {
			t.Errorf("shelves[%d].TypeKey = %q, want %q", i, shelves[i].TypeKey, want)
		}
		if shelves[i].LabelKey == "" || shelves[i].DescKey == "" {
			t.Errorf("shelves[%d] (%s) missing LabelKey/DescKey", i, want)
		}
	}
}

func TestWissenShelfFromTypeKey(t *testing.T) {
	if _, ok := WissenShelfFromTypeKey("bogus"); ok {
		t.Error("WissenShelfFromTypeKey(bogus) = ok, want not found")
	}
	shelf, ok := WissenShelfFromTypeKey("project")
	if !ok || shelf.TypeKey != "project" {
		t.Fatalf("WissenShelfFromTypeKey(project) = %+v, %v", shelf, ok)
	}
}

func TestWissenShelfForType_AgentFoldsIntoSpec(t *testing.T) {
	shelf, ok := WissenShelfForType(domain.DocAgent)
	if !ok || shelf.TypeKey != "spec" {
		t.Fatalf("WissenShelfForType(DocAgent) = %+v, %v, want spec shelf", shelf, ok)
	}
}

func TestWissenShelfForType_ContextFoldsThreeTypes(t *testing.T) {
	for _, typ := range []domain.DocumentType{domain.DocActiveContext, domain.DocInstruction, domain.DocSkill} {
		shelf, ok := WissenShelfForType(typ)
		if !ok || shelf.TypeKey != "context" {
			t.Fatalf("WissenShelfForType(%s) = %+v, %v, want context shelf", typ, shelf, ok)
		}
	}
}

func TestBuildWissenOverviewShelfCounts(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "p1", Type: domain.DocProject, Title: "Project", UpdatedAt: now, Pinned: true},
		{ID: "pl1", Type: domain.DocPlan, Title: "Plan", UpdatedAt: now},
		{ID: "s1", Type: domain.DocSpec, Title: "Spec", UpdatedAt: now},
		{ID: "a1", Type: domain.DocAgent, Title: "Legacy Agent", UpdatedAt: now}, // folds into spec
		{ID: "m1", Type: domain.DocMemory, Title: "Memory", UpdatedAt: now},
		{ID: "d1", Type: domain.DocDaily, Title: "Daily", UpdatedAt: now},
		{ID: "c1", Type: domain.DocActiveContext, Title: "Context", UpdatedAt: now},
		{ID: "i1", Type: domain.DocInstruction, Title: "Instruction", UpdatedAt: now}, // folds into context
		{ID: "f1", Type: domain.DocFree, Title: "Free", UpdatedAt: now},
	}
	vm := BuildWissenOverview(docs, now, false)

	if vm.TotalCount != len(docs) {
		t.Errorf("TotalCount = %d, want %d", vm.TotalCount, len(docs))
	}
	if vm.PinnedCount != 1 {
		t.Errorf("PinnedCount = %d, want 1", vm.PinnedCount)
	}

	counts := map[string]int{}
	for _, s := range vm.Shelves {
		counts[s.TypeKey] = s.Count
	}
	want := map[string]int{"project": 1, "plan": 1, "spec": 2, "memory": 1, "daily": 1, "context": 2, "free": 1}
	for typeKey, wantCount := range want {
		if counts[typeKey] != wantCount {
			t.Errorf("shelf %q count = %d, want %d", typeKey, counts[typeKey], wantCount)
		}
	}
}

func TestBuildWissenOverviewRecentCapAndAll(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	var docs []domain.Document
	for i := 0; i < 10; i++ {
		docs = append(docs, domain.Document{
			ID: "d" + itoa(i), Type: domain.DocFree, Title: "Doc " + itoa(i),
			UpdatedAt: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	capped := BuildWissenOverview(docs, now, false)
	if len(capped.Recent) != wissenRecentCap {
		t.Fatalf("capped Recent = %d rows, want %d", len(capped.Recent), wissenRecentCap)
	}
	if capped.RecentTotal != 10 {
		t.Errorf("RecentTotal = %d, want 10", capped.RecentTotal)
	}
	if capped.Recent[0].Title != "Doc 0" {
		t.Errorf("Recent[0].Title = %q, want newest-first %q", capped.Recent[0].Title, "Doc 0")
	}

	all := BuildWissenOverview(docs, now, true)
	if len(all.Recent) != 10 {
		t.Fatalf("recentAll Recent = %d rows, want 10", len(all.Recent))
	}
	if !all.RecentAll {
		t.Error("RecentAll should be true when requested")
	}
}

func TestWissenRowFromDocument_MetaDegradesWithoutActor(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	withActor := domain.Document{
		ID: "d1", Type: domain.DocProject, Title: "Doc", Path: "alpha/note",
		UpdatedAt: now.Add(-time.Hour), UpdatedByKind: "human", UpdatedByRef: "Soenne",
	}
	row := WissenRowFromDocument(withActor, now)
	if row.Meta != "alpha/note · Soenne" {
		t.Errorf("Meta with actor = %q, want %q", row.Meta, "alpha/note · Soenne")
	}
	if row.ChipClass != DocTypeChipClass(domain.DocProject) || row.ChipLabel != DocTypeLabel(domain.DocProject) {
		t.Errorf("chip class/label not wired: %+v", row)
	}

	noActor := domain.Document{ID: "d2", Type: domain.DocFree, Title: "Doc2", Path: "free/idea", UpdatedAt: now}
	row2 := WissenRowFromDocument(noActor, now)
	if row2.Meta != "free/idea" {
		t.Errorf("Meta without actor = %q, want just the path %q", row2.Meta, "free/idea")
	}
}

func TestBuildWissenType(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	shelf, _ := WissenShelfFromTypeKey("daily")
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocDaily, Title: "Daily One", Path: "daily/2026-07-06", UpdatedAt: now},
	}
	vm := BuildWissenType(shelf, docs, now)
	if vm.Shelf.TypeKey != "daily" {
		t.Fatalf("Shelf not carried through: %+v", vm.Shelf)
	}
	if len(vm.Rows) != 1 || vm.Rows[0].Title != "Daily One" {
		t.Fatalf("Rows = %+v, want one row for Daily One", vm.Rows)
	}
}

func TestWissenSummary(t *testing.T) {
	ctx := testCtx(t)
	got := WissenSummary(ctx, WissenOverviewVM{TotalCount: 264, PinnedCount: 12})
	if got != "264 Dokumente · 12 angepinnt" {
		t.Errorf("WissenSummary = %q, want %q", got, "264 Dokumente · 12 angepinnt")
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

func TestSwatchStyle(t *testing.T) {
	if s := swatchStyle(""); s != "" {
		t.Errorf("swatchStyle('') = %q, want ''", s)
	}
	got := swatchStyle("#aabbcc")
	if got != "--swatch: #aabbcc" {
		t.Errorf("swatchStyle('#aabbcc') = %q, want '--swatch: #aabbcc'", got)
	}
}

// TestDocRowUnaffected pins that DocRow/docRowFromDocument (and their
// BuildHomeNewest caller) keep working unmodified — the L3 Task 7 Wissen
// redesign adds WissenRowVM as a separate display struct instead of
// reshaping DocRow, which the L4 Schreibtisch page (home_newest.go) still
// depends on (Codex #18).
func TestDocRowUnaffected(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Title: "Home Doc", Path: "free/idea", UpdatedAt: now},
	}
	rows := BuildHomeNewest(docs, nil, 5)
	if len(rows) != 1 || rows[0].ID != "d1" || rows[0].Title != "Home Doc" {
		t.Fatalf("BuildHomeNewest regressed: %+v", rows)
	}
}
