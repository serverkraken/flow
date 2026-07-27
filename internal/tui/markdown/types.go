package markdown

// NoteType enumerates the note categories the frontmatter card knows
// how to badge. Defined locally so the renderer doesn't reach into
// any caller's domain layer; map your domain's note type to one of
// these constants when building a Frontmatter.
type NoteType string

// Recognised note types. Values match the legacy kompendium frontmatter
// strings ("daily", "project", "free") so existing on-disk frontmatter
// round-trips without translation.
const (
	TypeDaily   NoteType = "daily"
	TypeProject NoteType = "project"
	TypeFree    NoteType = "free"
)

// Frontmatter is the renderer-facing shape of a note's frontmatter
// card. Pass via WithFrontmatter to render the styled card above the
// body. Empty fields are tolerated — the card layout collapses
// whatever metadata is set without breaking on missing fields.
type Frontmatter struct {
	ID      string
	Type    NoteType
	Project string
	Date    string
	Title   string
	Tags    []string
}

// IsEmpty reports whether the frontmatter has nothing worth rendering.
// The renderer short-circuits on the empty case to skip the card.
func (f Frontmatter) IsEmpty() bool {
	return f.ID == "" && f.Title == "" && f.Project == "" && len(f.Tags) == 0
}

// BacklinkRef is the renderer-facing shape of one backlink entry.
// Passed via WithBacklinks to render the "Referenced by" footer.
// Title may be empty; the renderer falls back to the ID in that case.
type BacklinkRef struct {
	ID    string
	Title string
}
