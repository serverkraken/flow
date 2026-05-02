package testutil

// FakeWikilinkResolver maps targets to canned (uri, title) pairs.
// Targets not in the map resolve to ok=false so the renderer reaches
// its broken-link path.
type FakeWikilinkResolver struct {
	Entries map[string]FakeWikilinkEntry
}

// FakeWikilinkEntry is the canned resolution returned by
// FakeWikilinkResolver for a known target.
type FakeWikilinkEntry struct {
	URI   string
	Title string
}

// Resolve implements ports.WikilinkResolver.
func (f *FakeWikilinkResolver) Resolve(target string) (string, string, bool) {
	if f.Entries == nil {
		return "", "", false
	}
	e, ok := f.Entries[target]
	if !ok {
		return "", "", false
	}
	return e.URI, e.Title, true
}
