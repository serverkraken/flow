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

// StatusBadge returns a German label and Tailwind chip classes for a node status.
func StatusBadge(s domain.NodeStatus) (label, classes string) {
	switch s {
	case domain.NodePaused:
		return "pausiert", "rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 opacity-70"
	case domain.NodeArchived:
		return "archiviert", "rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-400"
	default:
		return "aktiv", "rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700"
	}
}
