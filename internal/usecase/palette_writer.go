package usecase

import (
	"sync"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// PaletteWriter mutates the palette stats: bumping the dispatch count
// for an action (Mark) or toggling the pin bit. The stats load failure
// is tolerated — we start from an empty map so a missing file doesn't
// block recording.
//
// In-process writes are serialised via mu so two quick palette actions
// can't interleave a load-modify-save and lose one of the updates.
// Cross-process serialisation is out of scope here — palette stats are
// only mutated by interactive flow processes, never by the
// status-bar-frequency `flow worktime status` callers.
type PaletteWriter struct {
	Stats ports.PaletteStatsStore
	Clock ports.Clock

	mu sync.Mutex
}

// Mark records that e was just dispatched. Increments Count and updates
// LastUsed to clock-now.
func (w *PaletteWriter) Mark(e domain.PaletteEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	stats, _ := w.Stats.Load()
	stats.Mark(e, w.Clock.Now())
	return w.Stats.Save(stats)
}

// TogglePin flips the Pinned bit for e and persists the change.
func (w *PaletteWriter) TogglePin(e domain.PaletteEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	stats, _ := w.Stats.Load()
	stats.TogglePin(e)
	return w.Stats.Save(stats)
}
