// This file used to host the duplicate Palette struct + Tokyonight /
// Catppuccin literals + Themes registry that drove the markdown
// renderer's colours. P2 (docs/design-system-audit.md §2.4) merged
// those into the canonical theme package at
// internal/frontend/tui/theme — there is now a single Palette
// definition for the whole TUI.
//
// The file stays so import ordering and historical search (`git log
// --follow`) keep working; everything that lived here moved to:
//
//   - canonical.Palette       — internal/frontend/tui/theme/palette.go
//   - canonical.TokyonightNight, CatppuccinMocha — same file
//   - canonical.Themes registry                   — same file
//
// Package-level shortcut shims (Bg, Blue, …) live in theme.go.

package theme
