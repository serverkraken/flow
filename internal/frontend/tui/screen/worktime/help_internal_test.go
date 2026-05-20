package worktime

// White-box tests for the static help-section helpers and the
// history_edit pure helpers. These are tiny pure functions that the
// black-box handler suite can't reach because they aren't invoked from
// any keybind path under test today.

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestHelpSections_AggregatesAllTabs(t *testing.T) {
	t.Parallel()
	sections := Model{}.HelpSections()
	if len(sections) < 6 {
		t.Fatalf("expected at least one section per tab + menu, got %d", len(sections))
	}
	// Every section must have a non-empty title and at least one binding.
	for i, s := range sections {
		if s.Title == "" {
			t.Errorf("section %d has empty title", i)
		}
		if len(s.Keys) == 0 {
			t.Errorf("section %d (%q) has no key bindings", i, s.Title)
		}
	}
}

func TestHelpSectionsTabs_HasBackToPalette(t *testing.T) {
	t.Parallel()
	s := helpSectionsTabs()
	if s.Title == "" {
		t.Errorf("title must be set")
	}
	// The 'b' binding must reference the palette-back action.
	found := false
	for _, kv := range s.Keys {
		if kv[0] == "b" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `b` binding in tabs section, got %+v", s.Keys)
	}
}

func TestHelpSectionsWoche_HasNavigation(t *testing.T) {
	t.Parallel()
	out := helpSectionsWoche()
	if len(out) != 1 || out[0].Title == "" {
		t.Errorf("Woche section should be exactly one block with a title, got %+v", out)
	}
}

func TestHelpSectionsHistory_TwoSections(t *testing.T) {
	t.Parallel()
	out := helpSectionsHistory()
	if len(out) != 2 {
		t.Fatalf("History should have 2 sections (main + drill), got %d", len(out))
	}
	// The drill section title must say "Drill".
	if !contains(out[1].Title, "Drill") {
		t.Errorf("second section should be the drill block, got title %q", out[1].Title)
	}
}

func TestHelpSectionsFrei_AddAndSync(t *testing.T) {
	t.Parallel()
	out := helpSectionsFrei()
	if len(out) != 1 {
		t.Fatalf("Frei should produce a single section, got %d", len(out))
	}
	keys := map[string]bool{}
	for _, kv := range out[0].Keys {
		keys[kv[0]] = true
	}
	for _, k := range []string{"a", "B", "D"} {
		if !keys[k] {
			t.Errorf("Frei section should include key %q", k)
		}
	}
}

func TestHelpSectionsMenu_HasActionBindings(t *testing.T) {
	t.Parallel()
	s := helpSectionsMenu()
	if len(s.Keys) == 0 {
		t.Errorf("Menu section should list its actions")
	}
}

// — history_edit pure helpers —

func TestDrillAddDefaults_EmptySessions(t *testing.T) {
	t.Parallel()
	start, stop := drillAddDefaults(nil)
	if start != "09:00" || stop != "" {
		t.Errorf("empty defaults: (%q, %q) want (\"09:00\", \"\")", start, stop)
	}
}

func TestDrillAddDefaults_FromLastStop(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: day, Start: day.Add(8 * time.Hour), Stop: day.Add(10 * time.Hour)},
		{Date: day, Start: day.Add(10 * time.Hour), Stop: day.Add(13 * time.Hour)},
	}
	start, stop := drillAddDefaults(sessions)
	if start != "13:00" {
		t.Errorf("start should follow last Stop, got %q want 13:00", start)
	}
	if stop != "" {
		t.Errorf("stop should be left blank for the user, got %q", stop)
	}
}

func TestLastSessionIndexForDate_PerDayIndexing(t *testing.T) {
	t.Parallel()
	day1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	day2 := day1.AddDate(0, 0, 1)
	all := []domain.Session{
		{Date: day1, Start: day1.Add(8 * time.Hour)},
		{Date: day2, Start: day2.Add(9 * time.Hour)},
		{Date: day1, Start: day1.Add(13 * time.Hour)},
		{Date: day1, Start: day1.Add(15 * time.Hour)},
	}
	// Three day1 sessions → last index for day1 is 2.
	if got := lastSessionIndexForDate(all, day1); got != 2 {
		t.Errorf("day1 last index: got %d want 2", got)
	}
	// One day2 session → last index for day2 is 0.
	if got := lastSessionIndexForDate(all, day2); got != 0 {
		t.Errorf("day2 last index: got %d want 0", got)
	}
	// Missing day returns -1.
	other := day1.AddDate(0, 0, 7)
	if got := lastSessionIndexForDate(all, other); got != -1 {
		t.Errorf("missing day should be -1, got %d", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
