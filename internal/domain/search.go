package domain

// Highlight sentinels wrap matched spans in a search Snippet (emitted by the
// store's ts_headline StartSel/StopSel) and are replaced by each host: WebUI →
// <mark>…</mark>, TUI → a lipgloss highlight on/off. Control chars are used so
// they never collide with document text.
const (
	HighlightStart = "\x02"
	HighlightEnd   = "\x03"
)

// SearchHit is a document plus its search snippet. The Document is embedded
// anonymously so the JSON is flat (Document fields + "snippet") — a plain
// []Document decoder still works against a SearchHit response.
type SearchHit struct {
	Document
	Snippet string `json:"snippet"`
}
