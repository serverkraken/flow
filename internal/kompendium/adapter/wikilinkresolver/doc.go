// Package wikilinkresolver implements flow's ports.WikilinkResolver
// against the kompendium note store. The shared markdown renderer
// (internal/frontend/tui/markdown) consults the resolver so `[[id]]`
// wikilinks in a kompendium note render as OSC 8 hyperlinks pointing
// at kompendium://note/<id> URIs that the editor launcher knows how
// to act on. Targets that don't resolve to a note in the store render
// as broken links (ok=false).
package wikilinkresolver
