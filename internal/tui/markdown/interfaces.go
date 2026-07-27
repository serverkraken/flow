package markdown

// WikilinkResolver looks up `[[id]]` / `[[id|display]]` targets so the
// renderer can style them valid (OSC 8 hyperlink + accent) or broken (red
// marker). Returns ok=false when unknown. When ok=true, uri is the address
// the OSC 8 escape carries (the docs viewer uses flow://docs/<id>) and title
// is the fallback display when no `|display` override is given.
type WikilinkResolver interface {
	Resolve(target string) (uri string, title string, ok bool)
}
