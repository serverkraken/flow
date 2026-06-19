package markdown_overlay

// RenderFunc renders markdown source at the requested inner width.
// Callers bind their renderer (and any markdown.Options) into the
// closure, so the overlay component itself doesn't depend on either
// ports.MarkdownRenderer or the internal/frontend/tui/markdown package.
//
// Implementations should NEVER return "" for a non-empty source —
// empty signals "renderer not wired" and the overlay falls back to raw
// text. Render failures should surface as the raw source, not as an
// error: the user prefers an unrendered markdown wall over an empty
// overlay.
type RenderFunc func(src string, width int) string
