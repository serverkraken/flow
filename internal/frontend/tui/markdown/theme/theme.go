// Package theme is the markdown renderer's role-mapping layer over the
// canonical token package at internal/frontend/tui/theme.
//
// Architecture:
//
//   - The canonical Palette + Sem live in canonical theme/.
//   - This package builds the markdown-specific MarkdownRoles bundle
//     (H1Bar, CodeFenceBg, CardBadge*, …) from a Palette parameter.
//   - The package-level shortcut vars below (Bg, Blue, Cyan, …) are a
//     deprecated transitional shim for kompendium/browse/styles.go and
//     a few older call-sites that read them directly. They are
//     populated once from canonical.Default at init time and never
//     mutate. P4 of the design-system roadmap migrates those screens
//     onto per-call palette-as-parameter; once that lands the shims
//     can be deleted.
//
// What's gone (P2 — docs/design-system-audit.md §2.4 / §2.7):
//
//   - The duplicate Palette struct + Tokyonight / Catppuccin literals.
//     The canonical theme.Palette is the single source of truth.
//   - The Active package var and SetActive() runtime mutator.
//     MarkdownRolesFor takes a Palette parameter so a per-call swap
//     (e.g. NO_COLOR test) is parallel-safe — no global state.
//   - The Themes registry. Theme selection is the canonical package's
//     job (theme.Themes there).
//
// Adding a heading / callout / card style: extend MarkdownRoles in
// markdown.go and the MarkdownRolesFor builder.
package theme

import canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"

// Deprecated shortcut vars — derived from canonical.Default at init.
// Kept until kompendium screens migrate (P4). New code should reach
// for canonical.Palette fields directly via a function parameter.
//
// The grouping below mirrors the canonical Palette field order so a
// missed token is obvious in review.
//
//nolint:gochecknoglobals // intentional transitional shim, see package doc
var (
	// Surface
	Bg              = canonical.Default.Bg
	PanelBg         = canonical.Default.BgPanel
	BgCode          = canonical.Default.BgCode
	BgHighlight     = canonical.Default.BgChip
	BgHighlightSoft = canonical.Default.BgChipSoft
	BarBg           = canonical.Default.BgBar
	DangerBg        = canonical.Default.BgDanger
	SuccessBg       = canonical.Default.BgSuccess

	// Foreground neutrals
	Fg    = canonical.Default.Fg
	FgDim = canonical.Default.FgDim
	Muted = canonical.Default.FgMuted

	// Foreground accents (raw hues — components prefer canonical.Sem)
	Blue    = canonical.Default.Blue
	Cyan    = canonical.Default.Cyan
	Green   = canonical.Default.Green
	Purple  = canonical.Default.Purple
	Magenta = canonical.Default.Magenta
	Yellow  = canonical.Default.Yellow
	Orange  = canonical.Default.Orange
	Red     = canonical.Default.Red
	Teal    = canonical.Default.Teal

	// TagPalette — Hash-rotated tag chip background pool.
	TagPalette = canonical.Default.TagPalette
)
