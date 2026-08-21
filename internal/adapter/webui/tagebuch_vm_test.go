package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func dailyDoc(id string, date time.Time, body string) domain.Document {
	return domain.Document{ID: id, Type: domain.DocDaily, Path: domain.DailyPath(date), Date: &date, Body: body}
}

func TestBuildTagebuchVM_SelectsNewestByDefault(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		dailyDoc("old", now.AddDate(0, 0, -2), "yesterday-1 body"),
		dailyDoc("newest", now, "# Heading\ntoday's line"),
	}
	vm := BuildTagebuchVM(context.Background(), docs, "", now)

	if vm.Selected == nil || vm.Selected.ID != "newest" {
		t.Fatalf("expected newest note selected, got %+v", vm.Selected)
	}
	if len(vm.Notes) != 2 || !vm.Notes[0].Selected {
		t.Fatalf("newest row should be marked selected: %+v", vm.Notes)
	}
	if vm.Notes[0].Excerpt != "today's line" {
		t.Errorf("excerpt should skip heading lines: %q", vm.Notes[0].Excerpt)
	}
}

func TestBuildTagebuchVM_ExplicitSelection(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		dailyDoc("a", now.AddDate(0, 0, -1), "a"),
		dailyDoc("b", now, "b"),
	}
	vm := BuildTagebuchVM(context.Background(), docs, "a", now)
	if vm.Selected == nil || vm.Selected.ID != "a" {
		t.Fatalf("explicit selection ignored: %+v", vm.Selected)
	}
}

func TestBuildTagebuchVM_StreakCountsConsecutiveDays(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		dailyDoc("d0", now, "x"),
		dailyDoc("d1", now.AddDate(0, 0, -1), "x"),
		dailyDoc("d2", now.AddDate(0, 0, -2), "x"),
		dailyDoc("gap", now.AddDate(0, 0, -5), "x"), // breaks the streak
	}
	vm := BuildTagebuchVM(context.Background(), docs, "", now)
	if !strings.Contains(vm.StreakLabel, "3") {
		t.Errorf("streak should be 3 consecutive days, got %q", vm.StreakLabel)
	}
}

func TestBuildTagebuchVM_Empty(t *testing.T) {
	vm := BuildTagebuchVM(context.Background(), nil, "", time.Now())
	if vm.Selected != nil {
		t.Fatalf("empty docs should have no selection, got %+v", vm.Selected)
	}
	if len(vm.Notes) != 0 {
		t.Fatalf("expected no notes, got %+v", vm.Notes)
	}
}

func TestBuildTagebuchArchivVM_GroupsByYearAndMonth(t *testing.T) {
	docs := []domain.Document{
		dailyDoc("jul1", time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), "july entry"),
		dailyDoc("jul2", time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), "july entry 2"),
		dailyDoc("aug1", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "august entry"),
	}
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)

	vm := BuildTagebuchArchivVM(context.Background(), docs, 0, 0, now)
	if vm.Year != 2026 {
		t.Fatalf("default year should be the newest present, got %d", vm.Year)
	}
	if len(vm.Months) != 2 {
		t.Fatalf("expected 2 months with entries, got %+v", vm.Months)
	}
	if vm.Months[0].Label != "August" {
		t.Errorf("months should be newest-first, got %+v", vm.Months)
	}
	if len(vm.Entries) != 1 || vm.Entries[0].ID != "aug1" {
		t.Fatalf("default month should be the newest with entries: %+v", vm.Entries)
	}

	julyVM := BuildTagebuchArchivVM(context.Background(), docs, 2026, 7, now)
	if len(julyVM.Entries) != 2 {
		t.Fatalf("explicit month=7 should list both july entries: %+v", julyVM.Entries)
	}
	if julyVM.Entries[0].ID != "jul2" {
		t.Errorf("entries should be newest-first within the month: %+v", julyVM.Entries)
	}
}

func TestBuildTagebuchArchivVM_EmptyFallsBackToNow(t *testing.T) {
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	vm := BuildTagebuchArchivVM(context.Background(), nil, 0, 0, now)
	if vm.Year != 2026 {
		t.Fatalf("empty docs should fall back to now's year, got %d", vm.Year)
	}
	if len(vm.Months) != 0 || len(vm.Entries) != 0 {
		t.Fatalf("expected no months/entries, got %+v / %+v", vm.Months, vm.Entries)
	}
}

func TestMarkHighlights_WrapsQuoteAndSkipsMissing(t *testing.T) {
	body := RenderMarkdown("Morgens den Fix verifiziert, danach Pause.")
	hs := []domain.NodeHighlight{
		{ID: "h1", NodeID: "n1", Quote: "den Fix verifiziert"},
		{ID: "h2", NodeID: "n1", Quote: "not present anywhere"},
	}
	out := markHighlights(body, hs, func(string) string { return "blue" })
	if !strings.Contains(string(out), `<mark class="bg-blue/[.14]">den Fix verifiziert</mark>`) {
		t.Fatalf("quote not wrapped: %s", out)
	}
	if strings.Count(string(out), "<mark") != 1 {
		t.Fatalf("missing quote must not corrupt output: %s", out)
	}
}

func TestBuildTagebuchMarkierenVM_AssignmentsAndTally(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 11, 13, 40, 0, 0, time.UTC)
	doc := dailyDoc("d1", now, "Heimserver Platten getauscht heute.")
	eng := domain.Node{ID: "eng1", Name: "Privat", Kind: domain.KindEngagement, Color: "green"}
	vor := domain.Node{ID: "vor1", Name: "Lesesaal-Rebuild", Kind: domain.KindVorhaben, Color: "violet", ParentID: &eng.ID}
	nodesByID := map[string]domain.Node{eng.ID: eng, vor.ID: vor}
	highlights := []domain.NodeHighlight{
		{ID: "h1", NodeID: vor.ID, Quote: "Platten getauscht", CreatedAt: now},
	}
	targets := []domain.Node{vor, eng}
	monthHighlights := []domain.NodeHighlight{
		{ID: "h1", NodeID: vor.ID}, {ID: "h2", NodeID: vor.ID}, {ID: "h3", NodeID: eng.ID},
	}

	vm := BuildTagebuchMarkierenVM(ctx, doc, highlights, nodesByID, targets, monthHighlights)

	if len(vm.Assignments) != 1 || vm.Assignments[0].TargetName != "Lesesaal-Rebuild" || vm.Assignments[0].ParentLabel != "Privat" {
		t.Fatalf("assignment not built correctly: %+v", vm.Assignments)
	}
	if len(vm.NodeOptions) != 2 {
		t.Fatalf("expected 2 assignable targets, got %+v", vm.NodeOptions)
	}
	if len(vm.Tally) != 2 || vm.Tally[0].Name != "Lesesaal-Rebuild" || vm.Tally[0].CountLabel != "2" {
		t.Fatalf("tally should rank the vorhaben with 2 highlights first: %+v", vm.Tally)
	}
	if !strings.Contains(string(vm.Doc.BodyHTML), "<mark") {
		t.Errorf("detail body should carry the inline mark: %s", vm.Doc.BodyHTML)
	}
}

// TestDailyExcerpt_SkipsMarkupLines pins what the month list shows: the first
// line a human would read. The ported version skipped headings only, so a note
// that opens with a code fence or a frontmatter delimiter rendered "```" as
// its excerpt — visible on screen in the live walkthrough, and no test saw it.
func TestDailyExcerpt_SkipsMarkupLines(t *testing.T) {
	for _, c := range []struct {
		name, body, want string
	}{
		{"code fence first", "```\nkubectl get pods\n```\nDann weitergebaut.", "kubectl get pods"},
		{"heading then fence", "# Freitag\n\n```bash\nmake ci\n```\nCI war grün.", "make ci"},
		{"frontmatter", "---\ntype: daily\n---\n\nHeute am Karteikasten.", "Heute am Karteikasten."},
		{"horizontal rule", "***\n\nEs ging weiter.", "Es ging weiter."},
		{"plain prose wins", "Einfach nur Text.", "Einfach nur Text."},
		{"nothing readable", "```\n```", ""},
	} {
		if got := dailyExcerpt(c.body); got != c.want {
			t.Errorf("%s: dailyExcerpt = %q, want %q", c.name, got, c.want)
		}
	}
}
