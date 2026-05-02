package ports

import "github.com/serverkraken/flow/internal/domain"

// PaletteEntryReader reads palette actions from the user's enabled tmux
// plugins (~/.tmux/plugins/<plugin>/menu.entries). Adapters handle the
// file-format parsing; the use case orders, ranks, and presents the result.
type PaletteEntryReader interface {
	List() ([]domain.PaletteEntry, error)
}

// PaletteStatsStore persists palette usage stats (per-action count, last
// used, pinned flag). Backed by ~/.local/state/flow/palette-stats.json.
type PaletteStatsStore interface {
	Load() (domain.PaletteStats, error)
	Save(domain.PaletteStats) error
}
