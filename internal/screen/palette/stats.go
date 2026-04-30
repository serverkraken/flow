package palette

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"time"
)

// ActionStat tracks how often and when an entry was dispatched, plus whether
// the user has pinned it. Persisted as JSON under
// ~/.local/state/flow/palette-stats.json (XDG state).
type ActionStat struct {
	Count    int       `json:"count,omitempty"`
	LastUsed time.Time `json:"last_used,omitempty"`
	Pinned   bool      `json:"pinned,omitempty"`
}

// Stats is the in-memory store. Empty / missing file → zero-value, all
// operations no-op safely.
type Stats struct {
	path    string
	actions map[string]ActionStat
}

func statsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "flow", "palette-stats.json")
}

// LoadStats reads the persisted stats file. Returns a zero-state Stats when
// the file is missing or unreadable — palette-stats are best-effort UX
// metadata, never a failure source.
func LoadStats() *Stats {
	s := &Stats{path: statsPath(), actions: map[string]ActionStat{}}
	if s.path == "" {
		return s
	}
	f, err := os.Open(s.path)
	if err != nil {
		return s
	}
	defer f.Close() //nolint:errcheck
	_ = json.NewDecoder(f).Decode(&s.actions)
	return s
}

// Save persists current stats. Errors are returned but callers may ignore
// them; a failed save degrades to no-history without breaking the palette.
func (s *Stats) Save() error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(s.path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	return json.NewEncoder(f).Encode(s.actions)
}

// Key derives the stats-map key from an entry. Section+Label is more stable
// than Action — the displayed action string occasionally changes (popup
// flags, paths) while section+label tend to stay put.
func entryKey(e Entry) string { return e.Section + "\x00" + e.Label }

// Mark records a dispatch.
func (s *Stats) Mark(e Entry) {
	if s == nil {
		return
	}
	if s.actions == nil {
		s.actions = map[string]ActionStat{}
	}
	a := s.actions[entryKey(e)]
	a.Count++
	a.LastUsed = time.Now()
	s.actions[entryKey(e)] = a
}

// TogglePin flips the pinned bit.
func (s *Stats) TogglePin(e Entry) {
	if s == nil {
		return
	}
	if s.actions == nil {
		s.actions = map[string]ActionStat{}
	}
	a := s.actions[entryKey(e)]
	a.Pinned = !a.Pinned
	s.actions[entryKey(e)] = a
}

// IsPinned reports the pin bit. The original Section field on the entry is
// used here — pin lookup must work BEFORE the section override is applied.
func (s *Stats) IsPinned(e Entry) bool {
	if s == nil {
		return false
	}
	return s.actions[entryKey(e)].Pinned
}

// EffectiveSection returns "Favoriten" for pinned entries, the original
// section otherwise. The original Section field on Entry is never mutated
// — display and sort code call this helper to derive the working section.
func (s *Stats) EffectiveSection(e Entry) string {
	if s.IsPinned(e) {
		return "Favoriten"
	}
	return e.Section
}

// score combines frequency and recency into a single sort key. Decays over
// days so a once-popular but now-unused action drifts back down without ever
// disappearing entirely.
func (s *Stats) score(e Entry) float64 {
	if s == nil {
		return 0
	}
	a := s.actions[entryKey(e)]
	if a.Count == 0 {
		return 0
	}
	ageDays := time.Since(a.LastUsed).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	recency := 1.0 / (1.0 + ageDays)
	return float64(a.Count) * (0.5 + recency)
}

// nearlyEqual avoids float-comparison instability in the sort comparator.
func nearlyEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
