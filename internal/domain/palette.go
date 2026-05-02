package domain

import (
	"math"
	"sort"
	"time"
)

// PaletteEntry is a single palette action loaded from a plugin's
// menu.entries file.
type PaletteEntry struct {
	Icon    string
	Label   string
	Action  string
	Section string
	Keybind string
	// Order is the original-input position of this entry across all
	// plugin files; lower values render first within the same section.
	Order int
}

// PaletteActionStat tracks how often and when a palette entry was
// dispatched, plus whether the user has pinned it. Persisted as JSON
// under ~/.local/state/flow/palette-stats.json.
type PaletteActionStat struct {
	Count    int       `json:"count,omitempty"`
	LastUsed time.Time `json:"last_used,omitempty"`
	Pinned   bool      `json:"pinned,omitempty"`
}

// PaletteStats is the per-action statistics map keyed by Section+Label.
// New zero-value maps are valid: every read method handles a nil Actions
// map without panicking.
type PaletteStats struct {
	Actions map[string]PaletteActionStat `json:"actions,omitempty"`
}

// EntryKey returns the stable key used in PaletteStats.Actions for an
// entry. Section+Label is more durable than Action — the action string
// occasionally changes (popup flags, paths) while section+label stay put.
func EntryKey(e PaletteEntry) string { return e.Section + "\x00" + e.Label }

// Mark records a dispatch of e at `now`: count++, LastUsed = now.
// Operates on the receiver in place; pointer receiver so a freshly-
// loaded zero-value stats receiver can also be marked.
func (s *PaletteStats) Mark(e PaletteEntry, now time.Time) {
	if s.Actions == nil {
		s.Actions = map[string]PaletteActionStat{}
	}
	key := EntryKey(e)
	a := s.Actions[key]
	a.Count++
	a.LastUsed = now
	s.Actions[key] = a
}

// TogglePin flips the Pinned bit for e.
func (s *PaletteStats) TogglePin(e PaletteEntry) {
	if s.Actions == nil {
		s.Actions = map[string]PaletteActionStat{}
	}
	key := EntryKey(e)
	a := s.Actions[key]
	a.Pinned = !a.Pinned
	s.Actions[key] = a
}

// IsPinned reports whether e is pinned. The lookup uses the entry's
// original section, never the "Favoriten" override returned by
// EffectiveSection — pinning must be idempotent across re-renders.
func (s PaletteStats) IsPinned(e PaletteEntry) bool {
	if s.Actions == nil {
		return false
	}
	return s.Actions[EntryKey(e)].Pinned
}

// EffectiveSection returns "Favoriten" for pinned entries and e.Section
// otherwise. Display + sort code uses this helper instead of e.Section
// directly so the original Section field never mutates.
func (s PaletteStats) EffectiveSection(e PaletteEntry) string {
	if s.IsPinned(e) {
		return "Favoriten"
	}
	return e.Section
}

// Score combines frequency and recency into one sort key. Recency decays
// over days so a once-popular but now-unused action drifts back down
// without disappearing entirely.
//
// score = count * (0.5 + 1/(1 + ageDays))  with ageDays = (now - LastUsed) / 24h
func (s PaletteStats) Score(e PaletteEntry, now time.Time) float64 {
	if s.Actions == nil {
		return 0
	}
	a := s.Actions[EntryKey(e)]
	if a.Count == 0 {
		return 0
	}
	ageDays := now.Sub(a.LastUsed).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	recency := 1.0 / (1.0 + ageDays)
	return float64(a.Count) * (0.5 + recency)
}

// paletteSectionOrder defines the priority of sections in the palette.
// Sections not listed here land after every listed one (rank len(order)).
var paletteSectionOrder = []string{
	"Favoriten",
	"Sidekick",
	"Kompendium",
	"Worktime",
	"Git",
	"Navigation",
	"System",
	"Misc",
}

// PaletteSectionRank returns the priority of section in the palette
// rendering. Lower values render first; unknown sections rank last.
func PaletteSectionRank(section string) int {
	for i, s := range paletteSectionOrder {
		if s == section {
			return i
		}
	}
	return len(paletteSectionOrder)
}

// SortPaletteEntries sorts entries in-place by:
//
//  1. Effective section rank (pinned entries surface to "Favoriten").
//  2. Score (frequency + recency) descending.
//  3. Original Order — stable tiebreaker preserving the menu.entries layout.
//
// `now` is used for recency decay in the score.
func SortPaletteEntries(entries []PaletteEntry, stats PaletteStats, now time.Time) {
	sort.SliceStable(entries, func(i, j int) bool {
		si := stats.EffectiveSection(entries[i])
		sj := stats.EffectiveSection(entries[j])
		ri, rj := PaletteSectionRank(si), PaletteSectionRank(sj)
		if ri != rj {
			return ri < rj
		}
		scoreI := stats.Score(entries[i], now)
		scoreJ := stats.Score(entries[j], now)
		if !nearlyEqual(scoreI, scoreJ) {
			return scoreI > scoreJ
		}
		return entries[i].Order < entries[j].Order
	})
}

// nearlyEqual avoids float-comparison instability in the sort comparator.
func nearlyEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
