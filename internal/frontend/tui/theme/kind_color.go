package theme

import (
	"image/color"

	"github.com/serverkraken/flow/internal/domain"
)

// KindColor maps a day-off Kind to its canonical Sem-token colour.
// Single source of truth across every screen and the tmux status segment:
//
//   - In-app TUI (week, history-heatmap, history-month, dayoffs picker)
//     calls KindColor directly to get a lipgloss colour for inline styling.
//   - tmux status-right (domain.KindStatusColor) projects the same Sem
//     tokens through StatusPaletteFor → StatusPalette so the hex value
//     that lands on the bar is identical to the in-app colour.
//
// Mapping (Spec 2026-05-13-filled-dayoff-dots-supersede):
//
//	Holiday   → Sem.Schedule  (fixed scheduled calendar event)
//	Vacation  → Sem.Highlight (chosen identity)
//	Sick      → Sem.Notice    (off-pattern warning, softer than Danger)
//	Unknown   → Fg            (renderable but uncoloured fallback)
//
// Adding a Kind requires one edit here plus the parallel slot in
// domain.KindStatusColor — the two functions are intentionally kept in
// lockstep, not collapsed, because the tmux side returns hex strings
// while the in-app side returns a theme.Color (which satisfies
// image/color.Color for direct lipgloss consumption).
func KindColor(p Palette, k domain.Kind) color.Color {
	sem := p.Sem()
	switch k {
	case domain.KindHoliday:
		return sem.Schedule
	case domain.KindVacation:
		return sem.Highlight
	case domain.KindSick:
		return sem.Notice
	}
	return p.Fg
}
