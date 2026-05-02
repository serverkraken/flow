// Package glamourrenderer implements ports.MarkdownRenderer using
// charmbracelet/glamour. The "dark" style is hardcoded — flow runs
// inside tmux against a tokyonight palette and consistent rendering
// matters more than per-instance theming.
//
// Render falls back to the raw Markdown when glamour can't be
// constructed, so the cheatsheet screen never loses the content.
package glamourrenderer
