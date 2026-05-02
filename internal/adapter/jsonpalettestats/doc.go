// Package jsonpalettestats implements ports.PaletteStatsStore as a
// JSON file (typically ~/.local/state/flow/palette-stats.json).
//
// The on-disk shape is the bare map of EntryKey → PaletteActionStat —
// the same layout the legacy screen/palette/stats.go writes — so
// existing user data is read unchanged. The wrapping
// domain.PaletteStats struct only exists in memory.
//
// A missing file yields an empty PaletteStats with no error: stats
// are best-effort UX metadata, never a failure source.
package jsonpalettestats
