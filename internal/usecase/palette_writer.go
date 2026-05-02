package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// PaletteWriter mutates the palette stats: bumping the dispatch count
// for an action (Mark) or toggling the pin bit. The stats load failure
// is tolerated — we start from an empty map so a missing file doesn't
// block recording.
type PaletteWriter struct {
	Stats ports.PaletteStatsStore
	Clock ports.Clock
}

// Mark records that e was just dispatched. Increments Count and updates
// LastUsed to clock-now.
func (w *PaletteWriter) Mark(e domain.PaletteEntry) error {
	stats, _ := w.Stats.Load()
	stats.Mark(e, w.Clock.Now())
	return w.Stats.Save(stats)
}

// TogglePin flips the Pinned bit for e and persists the change.
func (w *PaletteWriter) TogglePin(e domain.PaletteEntry) error {
	stats, _ := w.Stats.Load()
	stats.TogglePin(e)
	return w.Stats.Save(stats)
}
