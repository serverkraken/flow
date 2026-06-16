// Package theme is flow's single source of truth for visual tokens —
// colors, spacing, layout, and the style builders that turn them into
// rendered ANSI strings. Two consumers sit on top of it:
//
//   - markdown/theme — markdown role map (decoupled from globals; takes
//     a Palette argument).
//   - every screen and component package, which consume Palette directly
//     plus the canonical builders (Heading / Dim / Heading / …).
//
// The package keeps its imports light: lipgloss (for Color parsing) and
// stdlib only. tmux option lookup lives in load.go.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Color is a hex-string color (e.g. "#1a1b26") that satisfies the
// image/color.Color interface so it can be passed directly into any
// lipgloss style (Foreground, Background, BorderForeground, …) without
// a wrapping cast.
//
// The underlying string type is preserved deliberately: contrast tests
// and the tmux status adapter need the raw hex (string(p.Bg)) to compute
// luminance and to project palette tokens into shell escape sequences.
type Color string

// RGBA implements image/color.Color by delegating to the lipgloss parser,
// which understands hex (#rgb / #rrggbb) and ANSI index forms equally.
func (c Color) RGBA() (r, g, b, a uint32) {
	return lipgloss.Color(string(c)).RGBA()
}

// Palette is the canonical color set for one theme. Surface tokens
// (Bg, BgPanel, …) sit at the top; foreground tokens (Fg, FgDim,
// FgMuted) name the text stages; raw hues (Blue, Cyan, …) carry no
// semantic meaning by themselves — components consume them via Sem()
// instead, so a future palette swap doesn't drag every call-site.
type Palette struct {
	Name string

	// Surface — backgrounds, dark to light.
	Bg         Color // main stage
	BgPanel    Color // sub-panel
	BgCode     Color // code block fill / panel border
	BgChip     Color // selection / highlight fill
	BgChipSoft Color // alternating row tint
	BgBar      Color // heading / status-bar fill
	BgDanger   Color // callout-danger fill
	BgSuccess  Color // callout-success fill

	// Foreground — text stages, bright to dim.
	Fg      Color // body
	FgDim   Color // secondary
	FgMuted Color // hint / meta — never load-bearing content

	// Hue — raw color points. No semantic meaning.
	Blue, Cyan, Green, Purple, Magenta, Yellow, Orange, Red, Teal Color

	// TagPalette is rotated for hash-based chip coloring so a busy
	// tag-set doesn't read as a wall of one color. Order keeps adjacent
	// hues distinct.
	TagPalette []Color
}

// TokyonightNight — canonical default per docs/design-system-audit.md
// §2.1 (one stage). Bg is Tokyonight Night #1a1b26; Storm (#24283b) is
// dropped from the codebase as a separate fallback. Per-machine tmux
// overrides via @tn_* user-options can still select Storm at runtime.
//
// FgMuted is brighter than upstream Tokyonight `comment` (#565f89) so
// (FgMuted, Bg) clears WCAG AA — see contrast_test.go.
var TokyonightNight = Palette{
	Name:       "tokyonight-night",
	Bg:         "#1a1b26",
	BgPanel:    "#16161e",
	BgCode:     "#414868",
	BgChip:     "#3b4261",
	BgChipSoft: "#24283b",
	BgBar:      "#2a2e3f",
	BgDanger:   "#3b1f2b",
	BgSuccess:  "#1f3b2b",
	Fg:         "#c0caf5",
	FgDim:      "#a9b1d6",
	FgMuted:    "#9aa5ce",
	Blue:       "#7aa2f7",
	Cyan:       "#7dcfff",
	Green:      "#9ece6a",
	Purple:     "#bb9af7",
	Magenta:    "#ff007c",
	Yellow:     "#e0af68",
	Orange:     "#ff9e64",
	Red:        "#f7768e",
	Teal:       "#73daca",
	TagPalette: []Color{"#7dcfff", "#73daca", "#bb9af7", "#9ece6a", "#ff9e64", "#7aa2f7", "#e0af68"},
}

// CatppuccinMocha — second canonical palette. Mappings match upstream
// Mocha (https://github.com/catppuccin/catppuccin); FgMuted lifted from
// `subtext0` (#a6adc8) instead of `overlay0` (#6c7086) so contrast
// against Bg passes AA.
var CatppuccinMocha = Palette{
	Name:       "catppuccin-mocha",
	Bg:         "#1e1e2e",
	BgPanel:    "#181825",
	BgCode:     "#313244",
	BgChip:     "#45475a",
	BgChipSoft: "#292c3c",
	BgBar:      "#313244",
	BgDanger:   "#412a36",
	BgSuccess:  "#2a3b2f",
	Fg:         "#cdd6f4",
	FgDim:      "#bac2de",
	FgMuted:    "#a6adc8",
	Blue:       "#89b4fa",
	Cyan:       "#94e2d5",
	Green:      "#a6e3a1",
	Purple:     "#cba6f7",
	Magenta:    "#f5c2e7",
	Yellow:     "#f9e2af",
	Orange:     "#fab387",
	Red:        "#f38ba8",
	Teal:       "#94e2d5",
	TagPalette: []Color{"#94e2d5", "#a6e3a1", "#cba6f7", "#fab387", "#89b4fa", "#f9e2af", "#f5c2e7"},
}

// Themes is the registry of canonical palettes. Keys match Palette.Name.
var Themes = map[string]Palette{
	TokyonightNight.Name: TokyonightNight,
	CatppuccinMocha.Name: CatppuccinMocha,
}

// Default is the palette returned when no explicit selection is made.
// Wrappers (components/theme.Load) overlay tmux @tn_* options on top.
var Default = TokyonightNight

// Compile-time guard: every theme.Color must satisfy color.Color so it
// drops straight into lipgloss v2 style setters.
var _ color.Color = Color("")
