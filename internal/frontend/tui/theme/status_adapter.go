package theme

import "github.com/serverkraken/flow/internal/domain"

// StatusPaletteFor projects a canonical Palette onto the 5-field
// domain.StatusPalette used by the tmux status-right composer. The
// composer ultimately layers @tn_* tmux options on top of the result;
// callers wanting "canonical defaults, no overrides" can use this
// function as-is.
//
// Mapping:
//
//	Green  → Sem.Success  (status-bar achievement / streak)
//	Yellow → Sem.Warning  (active-session warning, mild)
//	Red    → Sem.Danger   (active-session over threshold, hard)
//	Cyan   → Sem.Info     (day-off banner, day-off pace dot)
//	Dim    → FgMuted      (idle banner, missed-target dot)
func StatusPaletteFor(p Palette) domain.StatusPalette {
	sem := p.Sem()
	return domain.StatusPalette{
		Green:  string(sem.Success),
		Yellow: string(sem.Warning),
		Red:    string(sem.Danger),
		Cyan:   string(sem.Info),
		Dim:    string(p.FgMuted),
	}
}
