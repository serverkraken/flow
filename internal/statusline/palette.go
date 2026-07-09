package statusline

// StatusPalette is the colour set used by tmux #[fg=...] markers in the
// status-right segment. Hex codes match the tokyonight defaults flow ships.
// Ported from the old domain/status.go; adapters override slots from tmux @tn_*.
type StatusPalette struct {
	Green, Yellow, Red, Cyan, Blue, Purple, Orange, Dim string
}

// DefaultStatusPalette returns the tokyonight defaults.
func DefaultStatusPalette() StatusPalette {
	return StatusPalette{
		Green: "#9ece6a", Yellow: "#e0af68", Red: "#f7768e", Cyan: "#7dcfff",
		Blue: "#7aa2f7", Purple: "#bb9af7", Orange: "#ff9e64", Dim: "#565f89",
	}
}

// Dimmed returns a copy whose every colour slot is Dim — the stale/offline
// render path feeds this so the whole segment reads as one muted "last known"
// snapshot (Spec §2 Stale+Dim). One-place mapping so a slot never leaks a live
// colour on the offline path.
func (p StatusPalette) Dimmed() StatusPalette {
	return StatusPalette{
		Green: p.Dim, Yellow: p.Dim, Red: p.Dim, Cyan: p.Dim,
		Blue: p.Dim, Purple: p.Dim, Orange: p.Dim, Dim: p.Dim,
	}
}
