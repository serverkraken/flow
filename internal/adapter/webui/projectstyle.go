package webui

import "github.com/serverkraken/flow/internal/domain"

// colorHex maps each domain.NodeColors name to its Tokyonight-Night hex.
// MUST cover every name in domain.NodeColors (enforced by a drift-guard test).
var colorHex = map[string]string{
	"blue":    "#7aa2f7",
	"cyan":    "#7dcfff",
	"green":   "#9ece6a",
	"purple":  "#bb9af7",
	"magenta": "#ff007c",
	"yellow":  "#e0af68",
	"orange":  "#ff9e64",
	"red":     "#f7768e",
	"teal":    "#73daca",
}

// ColorHex returns the swatch hex for a whitelisted color name, or "" for unset
// or unknown (the caller renders no swatch rather than guessing).
func ColorHex(name string) string { return colorHex[name] }

// StatusBadge returns a German label and Tailwind chip classes for a project
// status. Paused is dimmed; archived is muted; active is green-ish.
func StatusBadge(s domain.NodeStatus) (label, classes string) {
	switch s {
	case domain.NodePaused:
		return "pausiert", "rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 opacity-70"
	case domain.NodeArchived:
		return "archiviert", "rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-400"
	default: // active
		return "aktiv", "rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700"
	}
}
