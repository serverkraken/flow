package webui

// SearchRow is one Wissen bigsearch hit rendered as a Lesesaal row: the type
// chip (DocTypeChipClass/DocTypeLabel), title, path, and the rendered
// snippet HTML (server-escaped with <mark> highlights, see renderSnippet).
type SearchRow struct {
	ID, Title, ChipClass, ChipLabel, Path, Snippet string
	Archived                                       bool
	ContextEligible                                bool
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

// EmbedView is the embedding-status chip shown on a document.
type EmbedView struct {
	State     string
	LastError string
	ShowRetry bool
}
