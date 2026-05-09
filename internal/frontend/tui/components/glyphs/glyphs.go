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
	Info      = "›" // toast / status — informative without action
	BulletDot = "·" // mid-dot bullet (separator, low-key marker)
	BarThick  = "▌" // heading-bar / accent-block (one cell)
)

// Heat-cell glyphs — used by the history month-grid + heatmap to map
// quantitative load (pct of target) onto visual density. Memory bank
// (feedback_no_icons.md) explicitly whitelists ░▒▓█ — they are
// solid block-drawing, single-cell, no emoji-width drift.
const (
	HeatFull   = "█" // ≥100 % — solid block
	HeatDark   = "▓" // ≥75 %  — dark shade
	HeatMedium = "▒" // ≥50 %  — medium shade
	HeatLight  = "░" // >0 %   — light shade
)

// Markdown glyphs — used by the markdown renderer for list bullets,
// task checkboxes, and heading bars. Same single-cell, monospace-only,
// no-emoji rules as the rest of the whitelist (audit §2.1). Lives in
// this package so the cell-width test guards them too — the design-
// system audit treats markdown rendering as part of the canonical TUI,
// not a side-channel that gets to ignore the alignment rules.
const (
	Bullet1  = "●" // L1 list bullet (matches Filled — deliberate)
	Bullet2  = "○" // L2 list bullet (matches Empty — deliberate)
	Bullet3  = "◆" // L3 list bullet
	Bullet4  = "▪" // L4+ list bullet
	TaskOpen = "☐" // unchecked task
	TaskDone = "☑" // completed task
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
