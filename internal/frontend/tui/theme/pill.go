package theme

import "charm.land/lipgloss/v2"

// PillKind selects the pill's semantic flavour. Each kind carries a
// glyph in addition to its colour so the pill is readable in NO_COLOR
// or for colour-blind readers (audit A11y-2). docs/design-system-
// audit.md §2.3.2.
type PillKind int

const (
	// PillNeutral — palette-dim, "·" glyph (unknown / n.a.).
	PillNeutral PillKind = iota
	// PillSuccess — green ✓ (OK / passed).
	PillSuccess
	// PillWarning — yellow ▲ (heads-up).
	PillWarning
	// PillDanger — red ✗ (failed / blocking).
	PillDanger
	// PillActive — cyan ▶ (running / in progress).
	PillActive
	// PillInfo — accent › (informational).
	PillInfo
	// PillSkip — dim ○ (skipped / not applicable).
	PillSkip
)

// pillSpec is the glyph + colour-resolver tuple for one kind. Bundled
// here so the kind switch stays a single, readable table.
type pillSpec struct {
	glyph string
	color func(p Palette) lipgloss.Color
}

var pillSpecs = map[PillKind]pillSpec{
	PillNeutral: {"·", func(p Palette) lipgloss.Color { return p.FgMuted }},
	PillSuccess: {"✓", func(p Palette) lipgloss.Color { return p.Green }},
	PillWarning: {"▲", func(p Palette) lipgloss.Color { return p.Yellow }},
	PillDanger:  {"✗", func(p Palette) lipgloss.Color { return p.Red }},
	PillActive:  {"▶", func(p Palette) lipgloss.Color { return p.Cyan }},
	PillInfo:    {"›", func(p Palette) lipgloss.Color { return p.Sem().Accent }},
	PillSkip:    {"○", func(p Palette) lipgloss.Color { return p.FgMuted }},
}

// RenderPill returns "{glyph} {label}" coloured by kind. The visible
// width grows with the label; callers wanting columnar alignment pad
// at the call site. PillWidth = 4 is the minimum guideline (a
// glyph-only pill renders at width 1; pad to 4 for status-bar rows
// that have other 4-cell pills nearby).
//
// Use this in new code; the legacy string-keyed Pill() below is kept
// for back-compat with existing call-sites.
func RenderPill(kind PillKind, label string, p Palette) string {
	spec, ok := pillSpecs[kind]
	if !ok {
		spec = pillSpecs[PillNeutral]
	}
	body := spec.glyph
	if label != "" {
		body += " " + label
	}
	return lipgloss.NewStyle().Foreground(spec.color(p)).Bold(true).Render(body)
}

// Pill renders a fixed-width (PillWidth, 4 cells) coloured status
// indicator from a legacy string state name. Width is enforced even
// when the state's label would naturally exceed it — back-compat with
// status-bar rows that rely on the 4-cell column rhythm.
//
// Known states and their colours:
//
//   - "OK"   → green
//   - "FAIL" → red
//   - "RUN"  → cyan
//   - "..."  → orange (warning hue)
//   - "skip" → dim
//
// All other values render in dim. Prefer RenderPill in new code so
// the call-site documents the semantic kind and isn't constrained to
// the 4-cell budget.
func Pill(state string, p Palette) string {
	c := pillStateColor(state, p)
	return lipgloss.NewStyle().Foreground(c).Bold(true).Width(PillWidth).Render(state)
}

func pillStateColor(state string, p Palette) lipgloss.Color {
	switch state {
	case "OK":
		return p.Green
	case "FAIL":
		return p.Red
	case "RUN":
		return p.Cyan
	case "...":
		return p.Orange
	}
	return p.FgMuted
}
