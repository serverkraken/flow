package cli_test

import (
	"testing"
)

// Brief's --scope flag and the positional argument disagree → error.
// Covers the conflict-detection branches in newBriefCmd's RunE that the
// existing TestBrief_* suite doesn't reach.

func TestBrief_ScopeFlagWeekConflictsWithMonthArg(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("brief", "--scope", "week", "month")
	if err == nil {
		t.Errorf("--scope=week + arg=month should conflict")
	}
}

func TestBrief_ScopeFlagMonthConflictsWithWeekArg(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("brief", "--scope", "month", "week")
	if err == nil {
		t.Errorf("--scope=month + arg=week should conflict")
	}
}

func TestBrief_ScopeFlagMonthAlone(t *testing.T) {
	f := newFixture()
	if _, _, err := f.run("brief", "--scope", "month"); err != nil {
		t.Errorf("--scope=month alone should succeed, got %v", err)
	}
}
