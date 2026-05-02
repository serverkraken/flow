package ports

// WikilinkResolver looks up `[[id]]` and `[[id|display]]` wikilink
// targets so the markdown renderer can decide whether to style them as
// valid (OSC 8 hyperlink + accent colour) or broken (red marker, no
// link). The renderer does not know what backs the resolver — flow's
// cheatsheet has none and wires no resolver, while the kompendium TUI
// uses a sqlite-index-backed implementation that returns
// kompendium://note/<id> URIs.
//
// Returns ok=false when the target is unknown. When ok=true, uri is
// the address an OSC 8 escape will carry, and title (optional) is the
// resolved display the renderer falls back to when no `|display`
// override was given in the wikilink syntax.
type WikilinkResolver interface {
	Resolve(target string) (uri string, title string, ok bool)
}
