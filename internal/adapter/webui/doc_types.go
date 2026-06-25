package webui

// SearchRow is one search hit: a doc row plus its rendered snippet HTML.
type SearchRow struct {
	DocRow
	Snippet string
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
	Tags         []string
	ProjectID    string
	ProjectColor string
}

// EmbedView is the embedding-status chip shown on a document.
type EmbedView struct {
	State     string
	LastError string
	ShowRetry bool
}
