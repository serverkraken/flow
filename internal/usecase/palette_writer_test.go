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

// TestPaletteWriter_LoadErrorAbortsWrite verifies that a transient
// stats-load failure does NOT silently overwrite the on-disk stats
// with an empty struct. The legitimate "file does not exist" case
// returns a zero-value stats struct (no error) by adapter contract;
// only real I/O errors propagate through Load and they must abort
// the write.
func TestPaletteWriter_LoadErrorAbortsWrite(t *testing.T) {
	loadErr := errors.New("file missing")
	store := &testutil.FakePaletteStatsStore{LoadErr: loadErr}
	w := &usecase.PaletteWriter{
		Stats: store,
		Clock: &testutil.FixedClock{T: time.Now()},
	}
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	if err := w.Mark(e); !errors.Is(err, loadErr) {
		t.Errorf("expected loadErr from Mark, got %v", err)
	}
	if err := w.TogglePin(e); !errors.Is(err, loadErr) {
		t.Errorf("expected loadErr from TogglePin, got %v", err)
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
