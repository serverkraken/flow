package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestEntryKey(t *testing.T) {
	a := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	b := domain.PaletteEntry{Section: "Worktime", Label: "Stop"}
	c := domain.PaletteEntry{Section: "Git", Label: "Start"}

	ka := domain.EntryKey(a)
	kb := domain.EntryKey(b)
	kc := domain.EntryKey(c)

	if ka == kb {
		t.Error("different labels should produce different keys")
	}
	if ka == kc {
		t.Error("different sections should produce different keys")
	}
	// The NUL separator means a section ending with the next entry's label
	// can't accidentally collide.
	tricky := domain.EntryKey(domain.PaletteEntry{Section: "Worktime\x00", Label: "x"})
	if tricky == ka {
		t.Error("NUL boundary should still disambiguate")
	}
}

func TestPaletteStats_IsPinnedNilMap(t *testing.T) {
	if (domain.PaletteStats{}).IsPinned(domain.PaletteEntry{}) {
		t.Error("nil Actions should report not pinned")
	}
}

func TestPaletteStats_EffectiveSection(t *testing.T) {
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}
	pinned := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
		domain.EntryKey(e): {Pinned: true},
	}}
	if got := pinned.EffectiveSection(e); got != "Favoriten" {
		t.Errorf("pinned should map to Favoriten, got %q", got)
	}
	if got := (domain.PaletteStats{}).EffectiveSection(e); got != "Worktime" {
		t.Errorf("unpinned should pass through, got %q", got)
	}
}

func TestPaletteStats_Score(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}

	t.Run("nil Actions returns 0", func(t *testing.T) {
		if got := (domain.PaletteStats{}).Score(e, now); got != 0 {
			t.Errorf("nil → got %v", got)
		}
	})

	t.Run("zero count returns 0", func(t *testing.T) {
		s := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 0},
		}}
		if got := s.Score(e, now); got != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("recent use scores higher than old", func(t *testing.T) {
		recent := now.Add(-time.Hour)
		old := now.AddDate(0, 0, -30)
		recentStats := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 5, LastUsed: recent},
		}}
		oldStats := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 5, LastUsed: old},
		}}
		if recentStats.Score(e, now) <= oldStats.Score(e, now) {
			t.Error("recent should outscore old at same count")
		}
	})

	t.Run("future LastUsed clamps age to 0", func(t *testing.T) {
		// Age is clamped, so future LastUsed gives the same score as now.
		future := now.Add(time.Hour)
		s := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 3, LastUsed: future},
		}}
		got := s.Score(e, now)
		// At ageDays=0 → recency=1; score = 3*(0.5+1) = 4.5.
		if got < 4.4 || got > 4.6 {
			t.Errorf("expected ~4.5, got %v", got)
		}
	})
}

func TestPaletteSectionRank(t *testing.T) {
	if domain.PaletteSectionRank("Favoriten") != 0 {
		t.Error("Favoriten should rank first")
	}
	if domain.PaletteSectionRank("Misc") <= domain.PaletteSectionRank("Worktime") {
		t.Error("Misc should rank after Worktime")
	}
	known := domain.PaletteSectionRank("Worktime")
	unknown := domain.PaletteSectionRank("DoesNotExist")
	if unknown <= known {
		t.Errorf("unknown section should rank last (%d), known Worktime=%d", unknown, known)
	}
}

func TestPaletteStats_Mark(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}

	t.Run("first mark initialises map", func(t *testing.T) {
		s := domain.PaletteStats{}
		s.Mark(e, now)
		got := s.Actions[domain.EntryKey(e)]
		if got.Count != 1 || !got.LastUsed.Equal(now) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("subsequent mark increments count", func(t *testing.T) {
		s := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 3, LastUsed: now.Add(-time.Hour)},
		}}
		s.Mark(e, now)
		got := s.Actions[domain.EntryKey(e)]
		if got.Count != 4 || !got.LastUsed.Equal(now) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("preserves Pinned across marks", func(t *testing.T) {
		s := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Count: 1, Pinned: true},
		}}
		s.Mark(e, now)
		if !s.Actions[domain.EntryKey(e)].Pinned {
			t.Error("Pinned should be preserved")
		}
	})
}

func TestPaletteStats_TogglePin(t *testing.T) {
	e := domain.PaletteEntry{Section: "Worktime", Label: "Start"}

	t.Run("toggles unpinned to pinned", func(t *testing.T) {
		s := domain.PaletteStats{}
		s.TogglePin(e)
		if !s.Actions[domain.EntryKey(e)].Pinned {
			t.Error("expected pinned after first toggle")
		}
	})

	t.Run("toggles pinned back to unpinned", func(t *testing.T) {
		s := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(e): {Pinned: true, Count: 5},
		}}
		s.TogglePin(e)
		got := s.Actions[domain.EntryKey(e)]
		if got.Pinned {
			t.Error("expected unpinned after second toggle")
		}
		if got.Count != 5 {
			t.Errorf("Count should be preserved, got %d", got.Count)
		}
	})
}

func TestSortPaletteEntries(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	worktimeStart := domain.PaletteEntry{Section: "Worktime", Label: "Start", Order: 0}
	worktimeStop := domain.PaletteEntry{Section: "Worktime", Label: "Stop", Order: 1}
	gitPull := domain.PaletteEntry{Section: "Git", Label: "Pull", Order: 2}
	miscX := domain.PaletteEntry{Section: "Misc", Label: "X", Order: 3}

	t.Run("section rank takes priority", func(t *testing.T) {
		entries := []domain.PaletteEntry{miscX, gitPull, worktimeStart}
		domain.SortPaletteEntries(entries, domain.PaletteStats{}, now)
		// Worktime ranks before Git ranks before Misc.
		want := []string{"Worktime", "Git", "Misc"}
		for i, w := range want {
			if entries[i].Section != w {
				t.Errorf("position %d: got %q, want %q", i, entries[i].Section, w)
			}
		}
	})

	t.Run("pinned entry surfaces to Favoriten", func(t *testing.T) {
		entries := []domain.PaletteEntry{worktimeStart, worktimeStop, gitPull}
		stats := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(gitPull): {Pinned: true},
		}}
		domain.SortPaletteEntries(entries, stats, now)
		// Pinned gitPull surfaces to Favoriten section, ranks first.
		if entries[0].Label != "Pull" {
			t.Errorf("pinned should surface first, got %+v", entries[0])
		}
	})

	t.Run("score breaks ties within section", func(t *testing.T) {
		entries := []domain.PaletteEntry{worktimeStart, worktimeStop}
		stats := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{
			domain.EntryKey(worktimeStop): {Count: 10, LastUsed: now},
		}}
		domain.SortPaletteEntries(entries, stats, now)
		if entries[0].Label != "Stop" {
			t.Errorf("higher score should rank first, got %+v", entries[0])
		}
	})

	t.Run("order breaks ties when scores equal", func(t *testing.T) {
		// Both unused → both score 0 → Order preserves the menu.entries
		// layout.
		entries := []domain.PaletteEntry{worktimeStop, worktimeStart}
		domain.SortPaletteEntries(entries, domain.PaletteStats{}, now)
		// Original Order is 0 (Start) < 1 (Stop), so Start should win.
		if entries[0].Label != "Start" {
			t.Errorf("Order tiebreaker failed, got %+v", entries[0])
		}
	})
}
