package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestPaletteWriter_Mark(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	store := &testutil.FakePaletteStatsStore{}
	w := &usecase.PaletteWriter{
		Stats: store,
		Clock: &testutil.FixedClock{T: now},
	}
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	if err := w.Mark(e); err != nil {
		t.Fatal(err)
	}
	got := store.Stats.Actions[domain.EntryKey(e)]
	if got.Count != 1 || !got.LastUsed.Equal(now) {
		t.Errorf("got %+v", got)
	}
	// Second mark increments.
	if err := w.Mark(e); err != nil {
		t.Fatal(err)
	}
	if store.Stats.Actions[domain.EntryKey(e)].Count != 2 {
		t.Errorf("Count = %d, want 2", store.Stats.Actions[domain.EntryKey(e)].Count)
	}
}

func TestPaletteWriter_TogglePin(t *testing.T) {
	store := &testutil.FakePaletteStatsStore{}
	w := &usecase.PaletteWriter{
		Stats: store,
		Clock: &testutil.FixedClock{T: time.Now()},
	}
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	if err := w.TogglePin(e); err != nil {
		t.Fatal(err)
	}
	if !store.Stats.Actions[domain.EntryKey(e)].Pinned {
		t.Error("expected pinned")
	}
	if err := w.TogglePin(e); err != nil {
		t.Fatal(err)
	}
	if store.Stats.Actions[domain.EntryKey(e)].Pinned {
		t.Error("expected unpinned after second toggle")
	}
}

func TestPaletteWriter_StatsLoadErrorIsTolerated(t *testing.T) {
	store := &testutil.FakePaletteStatsStore{LoadErr: errors.New("file missing")}
	w := &usecase.PaletteWriter{
		Stats: store,
		Clock: &testutil.FixedClock{T: time.Now()},
	}
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	if err := w.Mark(e); err != nil {
		t.Errorf("load error should be tolerated: %v", err)
	}
}

func TestPaletteWriter_SaveErrPropagates(t *testing.T) {
	store := &testutil.FakePaletteStatsStore{SaveErr: errors.New("disk full")}
	w := &usecase.PaletteWriter{
		Stats: store,
		Clock: &testutil.FixedClock{T: time.Now()},
	}
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	if err := w.Mark(e); err == nil {
		t.Error("expected save error")
	}
	if err := w.TogglePin(e); err == nil {
		t.Error("expected save error")
	}
}
