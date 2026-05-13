package theme

import "github.com/serverkraken/flow/internal/domain"

// StatusPaletteFor projects a canonical Palette onto the
// domain.StatusPalette used by the tmux status-right composer. The
// composer ultimately layers @tn_* tmux options on top of the result;
// callers wanting "canonical defaults, no overrides" can use this
// function as-is.
//
// Spec 2026-05-13-filled-dayoff-dots-supersede: Blue/Purple/Orange were
// added for the day-off pace dots (Holiday/Vacation/Sick) so they share
// the exact hue values with the in-app TUI kindColor mapping.
//
// Mapping:
//
//	Green  → Sem.Success     (status-bar achievement / streak)
//	Yellow → Sem.Warning     (Endspurt-Banner — transient warm accent)
//	Red    → Sem.Danger      (active-session over hard threshold)
//	Cyan   → Sem.Info        (live/running indicators)
//	Blue   → Sem.Schedule    (Feiertag pace dot — fixed scheduled off)
//	Purple → Sem.Highlight   (Urlaub pace dot — chosen identity)
//	Orange → Sem.Notice      (Krank pace dot — off-pattern warning)
//	Dim    → FgMuted         (idle banner, missed-target dot)
func StatusPaletteFor(p Palette) domain.StatusPalette {
	sem := p.Sem()
	return domain.StatusPalette{
		Green:  string(sem.Success),
		Yellow: string(sem.Warning),
		Red:    string(sem.Danger),
		Cyan:   string(sem.Info),
		Blue:   string(sem.Schedule),
		Purple: string(sem.Highlight),
		Orange: string(sem.Notice),
		Dim:    string(p.FgMuted),
	}
}
