// Package glyphs is the canonical TUI glyph whitelist used across
// flow. docs/design-system-audit.md §2.1 fixes the rules:
//
//   - monospace-only — every glyph occupies exactly one terminal cell.
//   - no emoji — emoji-width characters break tmux status segments and
//     nvim sidebar columns by overflowing one cell.
//   - no half-fill characters (◐, ◓, …) — some fonts render them at
//     emoji-width and the column rhythm goes off.
//
// Adding a glyph: only after agreeing it satisfies the rules above.
// The whitelist exists so a new screen can't sneak in a non-monospace
// glyph that breaks alignment elsewhere.
package glyphs

// State glyphs — work / activity status.
const (
	Active   = "▶" // running session, live, in-progress
	Stopped  = "■" // halted
	Paused   = "‖" // paused
	Done     = "✓" // succeeded / completed
	Failed   = "✗" // failed
	Up       = "▲" // increase / streak
	Down     = "▼" // decrease
	Filled   = "●" // achieved goal / today
	Empty    = "○" // missed goal / future
	Holiday  = "★" // public holiday
	Vacation = "☼" // vacation / personal-free
	Extra    = "✚" // extra / additional entry
)

// UI glyphs — selection, progress, structure.
const (
	AccentBar = "▎" // selection / focus indicator
	BarFilled = "▰" // progress bar — filled cell
	BarEmpty  = "▱" // progress bar — empty cell
)

// Box-drawing — single set used for every component border. Mixing
// with Unicode line-drawing variants (e.g. ┌─┐) is allowed when a
// component specifies it; the rounded set is the default.
const (
	BoxRoundedTL = "╭"
	BoxRoundedTR = "╮"
	BoxRoundedBL = "╰"
	BoxRoundedBR = "╯"

	BoxNormalTL = "┌"
	BoxNormalTR = "┐"
	BoxNormalBL = "└"
	BoxNormalBR = "┘"

	BoxDoubleTL = "╔"
	BoxDoubleTR = "╗"
	BoxDoubleBL = "╚"
	BoxDoubleBR = "╝"

	BoxHorizontal       = "─"
	BoxVertical         = "│"
	BoxHorizontalDouble = "═"
	BoxVerticalDouble   = "║"
)
