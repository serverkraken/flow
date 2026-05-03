// Package theme is the markdown renderer's role-mapping layer over
// the canonical token package at internal/frontend/tui/theme. It
// exposes:
//
//   - MarkdownRoles + MarkdownRolesFor — the per-style bundle keyed
//     by markdown element (H1Bar, CodeFenceBg, CardBadge*, …).
//   - CalloutKind constants + CalloutBadge / CalloutBar helpers for
//     GitHub-style alert blocks.
//
// Both APIs take the canonical theme.Palette as a parameter so a
// per-call swap (NO_COLOR test, per-screen palette override) is
// parallel-safe with no global state. See markdown.go.
//
// Migration history:
//
//   - P2 removed the duplicate Palette struct + Tokyonight/Catppuccin
//     literals + Active/SetActive globals; the canonical theme.Palette
//     became the single source of truth.
//   - P4 removed the deprecated package-level shortcut shims (theme.Bg,
//     theme.Blue, …) once the kompendium TUI screens stopped reading
//     them and switched to canonical.Default directly.
package theme
