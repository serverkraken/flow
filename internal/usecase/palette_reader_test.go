package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestPaletteReader_Load_SortsAndIncludesSession(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	worktime := domain.PaletteEntry{Section: "Worktime", Label: "Start", Order: 0}
	misc := domain.PaletteEntry{Section: "Misc", Label: "X", Order: 1}

	r := &usecase.PaletteReader{
		Entries: &testutil.FakePaletteEntryReader{Entries: []domain.PaletteEntry{misc, worktime}},
		Stats:   &testutil.FakePaletteStatsStore{},
		Tmux:    &testutil.FakeTmux{Session: "myproject"},
		Clock:   &testutil.FixedClock{T: now},
	}
	snap, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snap.SessionName != "myproject" {
		t.Errorf("SessionName = %q", snap.SessionName)
	}
	// Worktime ranks before Misc — input order should have been reversed.
	if len(snap.Entries) != 2 || snap.Entries[0].Section != "Worktime" {
		t.Errorf("entries not sorted: %+v", snap.Entries)
	}
}

func TestPaletteReader_Load_StatsErrorIsTolerated(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	r := &usecase.PaletteReader{
		Entries: &testutil.FakePaletteEntryReader{
			Entries: []domain.PaletteEntry{{Section: "Worktime", Label: "Start"}},
		},
		Stats: &testutil.FakePaletteStatsStore{LoadErr: errors.New("missing file")},
		Tmux:  &testutil.FakeTmux{},
		Clock: &testutil.FixedClock{T: now},
	}
	snap, err := r.Load()
	if err != nil {
		t.Fatalf("stats error should be tolerated: %v", err)
	}
	if len(snap.Entries) != 1 {
		t.Errorf("expected 1 entry, got %+v", snap.Entries)
	}
}

func TestPaletteReader_Load_EntriesErrorPropagates(t *testing.T) {
	r := &usecase.PaletteReader{
		Entries: &testutil.FakePaletteEntryReader{Err: errors.New("boom")},
		Stats:   &testutil.FakePaletteStatsStore{},
		Tmux:    &testutil.FakeTmux{},
		Clock:   &testutil.FixedClock{T: time.Now()},
	}
	if _, err := r.Load(); err == nil {
		t.Error("expected error from entries reader")
	}
}
