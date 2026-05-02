package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// PaletteReader loads the palette entries, ranks them by section + score
// + insertion order, and returns a snapshot the screen can render directly.
type PaletteReader struct {
	Entries ports.PaletteEntryReader
	Stats   ports.PaletteStatsStore
	Tmux    ports.Tmux
	Clock   ports.Clock
}

// PaletteSnapshot is the bundled output of a Load call. Sessions name is
// purely informational ("aktive Session: foo" header in the screen) — it
// has no influence on the ranking.
type PaletteSnapshot struct {
	Entries     []domain.PaletteEntry
	Stats       domain.PaletteStats
	SessionName string
}

// Load returns the sorted palette plus the persisted stats and the
// current tmux session name. Stats load failures are tolerated — an
// empty stats map yields a deterministic order without ranking boost.
func (r *PaletteReader) Load() (*PaletteSnapshot, error) {
	entries, err := r.Entries.List()
	if err != nil {
		return nil, err
	}
	stats, _ := r.Stats.Load()
	domain.SortPaletteEntries(entries, stats, r.Clock.Now())
	return &PaletteSnapshot{
		Entries:     entries,
		Stats:       stats,
		SessionName: r.Tmux.CurrentSessionName(),
	}, nil
}
