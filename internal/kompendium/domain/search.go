package domain

// SearchOrder controls how search results are sorted.
type SearchOrder int

// Defined SearchOrder values.
const (
	// OrderRelevance sorts results by FTS5 relevance score (default).
	OrderRelevance SearchOrder = iota
	// OrderRecent sorts results by note mtime, newest first.
	OrderRecent
)

// SearchQuery is the input shape for a notebook search.
type SearchQuery struct {
	Text    string
	Type    NoteType
	Project string
	Limit   int
	Order   SearchOrder
}

// IsEmpty reports whether the query has no text and no filters set.
func (q SearchQuery) IsEmpty() bool {
	return q.Text == "" && q.Type == "" && q.Project == ""
}

// SearchResult is one matched note returned by an Indexer.
type SearchResult struct {
	ID      ID
	Title   string
	Snippet string
	Score   float64
}

// LinkRef is a (target, title) pair returned by Indexer.BacklinksOf and
// LinksFrom. Carrying the title here lets the read view render
// `[[id|title]]`-style references without a second store lookup per
// link, which used to be N+1 in RenderBacklinks.
type LinkRef struct {
	ID    ID
	Title string
}
