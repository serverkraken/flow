package usecase

import (
	"sync"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// PaletteWriter mutates the palette stats: bumping the dispatch count
// for an action (Mark) or toggling the pin bit.
//
// Load errors are surfaced rather than swallowed: a transient read
// failure must not let the caller write back an empty {} that wipes
// every pin and history entry. The legitimate "file does not exist"
// case is a clean nil from PaletteStatsStore.Load by contract — the
// adapter returns a zero-value stats struct, not an error.
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
	stats, err := w.Stats.Load()
	if err != nil {
		return err
	}
	stats.Mark(e, w.Clock.Now())
	return w.Stats.Save(stats)
}

// TogglePin flips the Pinned bit for e and persists the change.
func (w *PaletteWriter) TogglePin(e domain.PaletteEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	stats, err := w.Stats.Load()
	if err != nil {
		return err
	}
	stats.TogglePin(e)
	return w.Stats.Save(stats)
}
