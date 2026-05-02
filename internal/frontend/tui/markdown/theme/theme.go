// Package theme is the palette + role mapping for the markdown
// renderer. Code-block syntax-highlight colours, callout badges,
// frontmatter card chips, and every other styled output the renderer
// emits resolve through the package-level shortcuts (Bg, Blue, Cyan, …)
// so a palette swap stays one SetActive call.
//
// The shortcut variables below mirror the active palette's fields.
// They are populated at init time from theme.Active (the default is
// Tokyonight); SetActive rewrites them when a caller picks a different
// theme. Consumers reference the unchanged identifiers (theme.Blue,
// theme.Bg, …) and never touch Active directly — but a palette swap
// propagates because every style is built per-render via
// MarkdownRolesFor.
package theme

// Backgrounds.
var (
	Bg              = Active.Bg
	PanelBg         = Active.PanelBg
	BgCode          = Active.BgCode
	BgHighlight     = Active.BgHighlight
	BgHighlightSoft = Active.BgHighlightSoft
	BarBg           = Active.BarBg
	DangerBg        = Active.DangerBg
	SuccessBg       = Active.SuccessBg
)

// Foregrounds — neutrals.
var (
	Fg    = Active.Fg
	FgDim = Active.FgDim
	Muted = Active.Muted
)

// Foregrounds — accents.
var (
	Blue    = Active.Blue
	Cyan    = Active.Cyan
	Green   = Active.Green
	Purple  = Active.Purple
	Magenta = Active.Magenta
	Yellow  = Active.Yellow
	Orange  = Active.Orange
	Red     = Active.Red
	Teal    = Active.Teal
)

// TagPalette is rotated for tag chips so a noisy tag-set doesn't read
// as a wall of one colour. Order is determined by the active theme.
var TagPalette = Active.TagPalette
