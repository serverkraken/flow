package domain

// Note is the in-memory representation of a single note in the notebook.
// It bundles its identifier, parsed frontmatter, and the markdown body.
type Note struct {
	ID   ID
	Meta Frontmatter
	Body []byte
}

// NewNote constructs a Note after validating its frontmatter. It returns an
// error wrapping ErrInvalidFrontmatter when the metadata is incoherent.
func NewNote(id ID, meta Frontmatter, body []byte) (Note, error) {
	if err := meta.Validate(); err != nil {
		return Note{}, err
	}
	return Note{ID: id, Meta: meta, Body: body}, nil
}

// Links returns every wikilink found in the note body, in document order.
func (n Note) Links() []Link {
	return ExtractLinks(n.Body)
}
