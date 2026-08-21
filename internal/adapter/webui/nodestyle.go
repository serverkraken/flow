package webui

import "github.com/serverkraken/flow/internal/domain"

// ColorHex returns a token-reactive CSS color expression for a whitelisted
// color name ("" for unset/unknown). rgb(var(--x)) flips with the theme —
// a fixed hex would freeze node swatches in one palette.
func ColorHex(name string) string {
	for _, n := range domain.NodeColors {
		if n == name {
			return "rgb(var(--" + name + "))"
		}
	}
	return ""
}

// StatusBadge returns a German label and token-based chip classes for a node
// status — Linie statt Pille, Töne aus dem Karteikasten (aktiv = live).
func StatusBadge(s domain.NodeStatus) (label, classes string) {
	switch s {
	case domain.NodePaused:
		return "pausiert", "border border-hair2 bg-sunken px-2 py-0.5 text-xs text-muted"
	case domain.NodeArchived:
		return "archiviert", "border border-hair2 bg-sunken px-2 py-0.5 text-xs text-faint"
	default:
		return "aktiv", "border border-live/25 bg-live-wash px-2 py-0.5 text-xs text-live"
	}
}
