package theme

// Padding — horizontal scale. Vertical padding in a TUI is almost
// always 0 or 1 line; we don't model it as a token because the choice
// is dictated by content (single-line chip vs multi-line modal).
const (
	PadNone = 0
	PadXS   = 1 // chip / pill / status-bar inner
	PadSM   = 2 // modal content L/R
	PadMD   = 3 // modal vertical, spaced sections
)

// Layout — recurring column widths. Hardcoded in screens today; lifted
// here so a width tweak is a one-line change.
const (
	PillWidth     = 4  // status pill (OK / RUN / FAIL …)
	KeyHintWidth  = 12 // key column in help overlays
	DayLabelWidth = 3  // weekday abbrev
	DateColWidth  = 9  // dd.mm.yyyy minus year-abbrev
	DefaultBox    = 60 // standard titlebox width
	NarrowBox     = 40 // popup, sidekick narrow pane
	WideBox       = 80 // full-width screens
)

// Layer is an ordering hint, not a real z-axis. Components use it to
// pick consistent border kinds and padding scales for "how prominent is
// this surface" — surface < panel < hover < selected < overlay < modal.
const (
	LayerSurface  = 0 // Bg
	LayerPanel    = 1 // BgPanel + NormalBorder
	LayerHover    = 2 // BgChipSoft
	LayerSelected = 3 // BgChip + accent bar
	LayerOverlay  = 4 // RoundedBorder
	LayerModal    = 5 // DoubleBorder + BgPanel + PadMD
)
