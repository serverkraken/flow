// Package markdown turns CommonMark + GFM source into ANSI-styled,
// terminal-ready text. The pipeline is goldmark for parsing, a custom
// node renderer (renderer.go + blocks.go + inline.go + code.go +
// table.go + footnote.go + frontmatter.go + backlinks.go + wikilink.go)
// for ANSI output, then a post-process WrapURLs (osc8.go) that wraps
// bare URLs as OSC 8 hyperlinks. Code blocks are syntax-highlighted via
// chroma; per-token colours come from the bundled theme subpackage.
//
// The renderer is shared across flow surfaces: the cheatsheet screen
// uses it without a resolver (no wikilinks in cheatsheets); the
// kompendium TUI passes a WikilinkResolver (ports.WikilinkResolver)
// that looks targets up against the sqlite index. Frontmatter and
// backlinks data shapes are defined locally (types.go) so the package
// stays decoupled from the kompendium domain/usecase layer.
package markdown
