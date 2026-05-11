// Package markdown_overlay is the shared Markdown reader component
// used by the worktime brief overlay, the worktime today-note overlay,
// and the kompendium full-screen viewer. It hosts a viewport, a
// re-flowing markdown body, an optional search mode, optional code-
// snippet copy, and a uniform chrome (rounded frame + title + separator
// + footer + status bar).
//
// Render abstraction is passed in as a RenderFunc closure so the
// component doesn't bind to a specific renderer (some callers use
// ports.MarkdownRenderer, others bind markdown.Render with options).
package markdown_overlay
