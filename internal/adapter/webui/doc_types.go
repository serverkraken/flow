package webui

// SearchRow is one Wissen bigsearch hit rendered as a Lesesaal row: the type
// chip (DocTypeChipClass/DocTypeLabel), title, path, and the rendered
// snippet HTML (server-escaped with <mark> highlights, see renderSnippet).
type SearchRow struct {
	ID, Title, ChipClass, ChipLabel, Path, Snippet string
}

// TagChip is one tag in the filter bar.
type TagChip struct {
	Tag    string
	Count  int
	Active bool
	Href   string
}

// TagLink pairs a tag string with a pre-encoded Wissen filter href.
type TagLink struct {
	Tag  string
	Href string
}

// DocRow is one document in a list.
type DocRow struct {
	ID           string
	Type         string
	Path         string
	Title        string
	Preview      string
	Tags         []string
	NodeID    string
	ProjectColor string
}

// EmbedView is the embedding-status chip shown on a document.
type EmbedView struct {
	State     string
	LastError string
	ShowRetry bool
}
