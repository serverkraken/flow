// Theme registry. Defines the Palette type, the bundled themes, and
// the SetActive switch that points the package-level shortcuts (Bg,
// Blue, Cyan, …) at a chosen palette.
//
// To add a theme:
//   1. Declare `var MyTheme = Palette{...}` below with the full
//      palette filled in.
//   2. Register it in the Themes map.
//   3. Activate at runtime with theme.SetActive(MyTheme).
//
// Tokyonight is the default; SetActive is the only switch. An env-var
// driven runtime selector is intentionally absent — flow's renderer is
// embedded across multiple surfaces (cheatsheet today, kompendium TUI
// in K5), and surface-specific palette overrides are deferred to the
// theme-consolidation decision in CLAUDE-kompendium-plan §K3.E.

package theme

// Palette is the colour ground truth for one theme. Every TUI surface
// (browse, view, write-picker, markdown renderer) reads its colours
// from the active palette via the package-level shortcuts below.
//
// Adding a field here means: pick a value for every Palette literal
// or the new field reads as the zero string for unmigrated themes.
type Palette struct {
	Name string

	// Backgrounds
	Bg              string // page bg
	PanelBg         string // pane bg (slightly different)
	BgCode          string // code-block panel bg
	BgHighlight     string // selected row / status bar bg
	BgHighlightSoft string // alternating-row tint
	BarBg           string // heading bar bg
	DangerBg        string // callout danger fill
	SuccessBg       string // callout success fill

	// Foregrounds — neutrals
	Fg    string
	FgDim string
	Muted string

	// Foregrounds — accents
	Blue    string
	Cyan    string
	Green   string
	Purple  string
	Magenta string
	Yellow  string
	Orange  string
	Red     string
	Teal    string

	// TagPalette is rotated for tag chips so a noisy tag-set doesn't
	// read as a wall of one colour. Order keeps adjacent hues distinct.
	TagPalette []string
}

// Tokyonight (Storm) — the default palette. Names follow the upstream
// convention so cross-referencing the Tokyonight repo stays trivial.
var Tokyonight = Palette{
	Name:            "tokyonight",
	Bg:              "#1a1b26",
	PanelBg:         "#16161e",
	BgCode:          "#414868",
	BgHighlight:     "#3b4261",
	BgHighlightSoft: "#24283b",
	BarBg:           "#2a2e3f",
	DangerBg:        "#3b1f2b",
	SuccessBg:       "#1f3b2b",
	Fg:              "#c0caf5",
	FgDim:           "#a9b1d6",
	Muted:           "#565f89",
	Blue:            "#7aa2f7",
	Cyan:            "#7dcfff",
	Green:           "#9ece6a",
	Purple:          "#bb9af7",
	Magenta:         "#ff007c",
	Yellow:          "#e0af68",
	Orange:          "#ff9e64",
	Red:             "#f7768e",
	Teal:            "#73daca",
	TagPalette:      []string{"#7dcfff", "#73daca", "#bb9af7", "#9ece6a", "#ff9e64", "#7aa2f7", "#e0af68"},
}

// Catppuccin (Mocha) — alternate dark palette. Provided as a worked
// example of the swap mechanism; mappings match the upstream Mocha
// flavour at https://github.com/catppuccin/catppuccin.
var Catppuccin = Palette{
	Name:            "catppuccin",
	Bg:              "#1e1e2e",
	PanelBg:         "#181825",
	BgCode:          "#313244",
	BgHighlight:     "#45475a",
	BgHighlightSoft: "#292c3c",
	BarBg:           "#313244",
	DangerBg:        "#412a36",
	SuccessBg:       "#2a3b2f",
	Fg:              "#cdd6f4",
	FgDim:           "#bac2de",
	Muted:           "#6c7086",
	Blue:            "#89b4fa",
	Cyan:            "#94e2d5",
	Green:           "#a6e3a1",
	Purple:          "#cba6f7",
	Magenta:         "#f5c2e7",
	Yellow:          "#f9e2af",
	Orange:          "#fab387",
	Red:             "#f38ba8",
	Teal:            "#94e2d5",
	TagPalette:      []string{"#94e2d5", "#a6e3a1", "#cba6f7", "#fab387", "#89b4fa", "#f9e2af", "#f5c2e7"},
}

// Themes is the registry consulted by env-var resolution. Keys are
// case-insensitive and match Palette.Name.
var Themes = map[string]Palette{
	"tokyonight": Tokyonight,
	"catppuccin": Catppuccin,
}

// Active is the currently-selected palette. Mutated by SetActive at
// startup; consumers that use the package-level shortcuts (Bg, Blue,
// …) automatically pick up the chosen theme without referencing
// Active directly.
var Active = Tokyonight

// SetActive points the package-level shortcuts at p and stores p as
// the active palette. Safe to call before any other package's init
// reads the shortcuts — Go init order runs theme's init first because
// every consumer imports it.
func SetActive(p Palette) {
	Active = p
	Bg = p.Bg
	PanelBg = p.PanelBg
	BgCode = p.BgCode
	BgHighlight = p.BgHighlight
	BgHighlightSoft = p.BgHighlightSoft
	BarBg = p.BarBg
	DangerBg = p.DangerBg
	SuccessBg = p.SuccessBg
	Fg = p.Fg
	FgDim = p.FgDim
	Muted = p.Muted
	Blue = p.Blue
	Cyan = p.Cyan
	Green = p.Green
	Purple = p.Purple
	Magenta = p.Magenta
	Yellow = p.Yellow
	Orange = p.Orange
	Red = p.Red
	Teal = p.Teal
	TagPalette = p.TagPalette
}
