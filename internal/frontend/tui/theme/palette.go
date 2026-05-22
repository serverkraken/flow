// Package theme is flow's single source of truth for visual tokens —
// colors, spacing, layout, and the style builders that turn them into
// rendered ANSI strings. Two consumers sit on top of it:
//
//   - markdown/theme — markdown role map (decoupled from globals; takes
//     a Palette argument).
//   - every screen and component package, which consume Palette directly
//     plus the canonical builders (Heading / Dim / Heading / …).
//
// The package keeps its imports light: lipgloss (for the typed Color
// alias and the renderer), termenv (for the TrueColor profile), and
// stdlib only. tmux option lookup lives in load.go.
package theme

import "charm.land/lipgloss/v2"

// Palette is the canonical color set for one theme. Surface tokens
// (Bg, BgPanel, …) sit at the top; foreground tokens (Fg, FgDim,
// FgMuted) name the text stages; raw hues (Blue, Cyan, …) carry no
// semantic meaning by themselves — components consume them via Sem()
// instead, so a future palette swap doesn't drag every call-site.
//
// All color fields are typed as lipgloss.Color so they can be passed
// directly to lipgloss.NewStyle().Foreground / .Background without an
// explicit cast at every call-site. lipgloss.Color is `type Color
// string`, so passing them to ContrastRatio (string-typed) needs a
// `string(p.Bg)` cast — an acceptable trade since contrast tests are
// few and screens are many.
type Palette struct {
	Name string

	// Surface — backgrounds, dark to light.
	Bg         lipgloss.Color // main stage
	BgPanel    lipgloss.Color // sub-panel
	BgCode     lipgloss.Color // code block fill / panel border
	BgChip     lipgloss.Color // selection / highlight fill
	BgChipSoft lipgloss.Color // alternating row tint
	BgBar      lipgloss.Color // heading / status-bar fill
	BgDanger   lipgloss.Color // callout-danger fill
	BgSuccess  lipgloss.Color // callout-success fill

	// Foreground — text stages, bright to dim.
	Fg      lipgloss.Color // body
	FgDim   lipgloss.Color // secondary
	FgMuted lipgloss.Color // hint / meta — never load-bearing content

	// Hue — raw color points. No semantic meaning.
	Blue, Cyan, Green, Purple, Magenta, Yellow, Orange, Red, Teal lipgloss.Color

	// TagPalette is rotated for hash-based chip coloring so a busy
	// tag-set doesn't read as a wall of one color. Order keeps adjacent
	// hues distinct.
	TagPalette []lipgloss.Color
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
	Bg:         lipgloss.Color("#1a1b26"),
	BgPanel:    lipgloss.Color("#16161e"),
	BgCode:     lipgloss.Color("#414868"),
	BgChip:     lipgloss.Color("#3b4261"),
	BgChipSoft: lipgloss.Color("#24283b"),
	BgBar:      lipgloss.Color("#2a2e3f"),
	BgDanger:   lipgloss.Color("#3b1f2b"),
	BgSuccess:  lipgloss.Color("#1f3b2b"),
	Fg:         lipgloss.Color("#c0caf5"),
	FgDim:      lipgloss.Color("#a9b1d6"),
	FgMuted:    lipgloss.Color("#9aa5ce"),
	Blue:       lipgloss.Color("#7aa2f7"),
	Cyan:       lipgloss.Color("#7dcfff"),
	Green:      lipgloss.Color("#9ece6a"),
	Purple:     lipgloss.Color("#bb9af7"),
	Magenta:    lipgloss.Color("#ff007c"),
	Yellow:     lipgloss.Color("#e0af68"),
	Orange:     lipgloss.Color("#ff9e64"),
	Red:        lipgloss.Color("#f7768e"),
	Teal:       lipgloss.Color("#73daca"),
	TagPalette: []lipgloss.Color{"#7dcfff", "#73daca", "#bb9af7", "#9ece6a", "#ff9e64", "#7aa2f7", "#e0af68"},
}

// CatppuccinMocha — second canonical palette. Mappings match upstream
// Mocha (https://github.com/catppuccin/catppuccin); FgMuted lifted from
// `subtext0` (#a6adc8) instead of `overlay0` (#6c7086) so contrast
// against Bg passes AA.
var CatppuccinMocha = Palette{
	Name:       "catppuccin-mocha",
	Bg:         lipgloss.Color("#1e1e2e"),
	BgPanel:    lipgloss.Color("#181825"),
	BgCode:     lipgloss.Color("#313244"),
	BgChip:     lipgloss.Color("#45475a"),
	BgChipSoft: lipgloss.Color("#292c3c"),
	BgBar:      lipgloss.Color("#313244"),
	BgDanger:   lipgloss.Color("#412a36"),
	BgSuccess:  lipgloss.Color("#2a3b2f"),
	Fg:         lipgloss.Color("#cdd6f4"),
	FgDim:      lipgloss.Color("#bac2de"),
	FgMuted:    lipgloss.Color("#a6adc8"),
	Blue:       lipgloss.Color("#89b4fa"),
	Cyan:       lipgloss.Color("#94e2d5"),
	Green:      lipgloss.Color("#a6e3a1"),
	Purple:     lipgloss.Color("#cba6f7"),
	Magenta:    lipgloss.Color("#f5c2e7"),
	Yellow:     lipgloss.Color("#f9e2af"),
	Orange:     lipgloss.Color("#fab387"),
	Red:        lipgloss.Color("#f38ba8"),
	Teal:       lipgloss.Color("#94e2d5"),
	TagPalette: []lipgloss.Color{"#94e2d5", "#a6e3a1", "#cba6f7", "#fab387", "#89b4fa", "#f9e2af", "#f5c2e7"},
}

// Themes is the registry of canonical palettes. Keys match Palette.Name.
var Themes = map[string]Palette{
	TokyonightNight.Name: TokyonightNight,
	CatppuccinMocha.Name: CatppuccinMocha,
}

// Default is the palette returned when no explicit selection is made.
// Wrappers (components/theme.Load) overlay tmux @tn_* options on top.
var Default = TokyonightNight
