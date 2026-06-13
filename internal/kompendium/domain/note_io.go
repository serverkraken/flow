package domain

// RenderNote serialises a Note to the on-disk/tempfile representation:
// YAML frontmatter delimited by "---" followed by the note body.
// The output is suitable for writing to a tempfile before handing it
// to an editor, and for roundtripping through ParseNote.
func RenderNote(n Note) []byte {
	return n.Meta.Serialize(n.Body)
}

// ParseNote deserialises raw bytes (as returned by os.ReadFile on a
// tempfile) back into a Note. id is used to populate Note.ID and to
// validate that the frontmatter id field matches if present.
//
// Returns ErrNoFrontmatter, ErrMalformedFrontmatter, or
// ErrInvalidFrontmatter (wrapped) on failure so callers can surface
// a useful error message.
func ParseNote(id ID, raw []byte) (Note, error) {
	fm, body, err := ParseFrontmatter(raw)
	if err != nil {
		return Note{}, err
	}
	return NewNote(id, fm, body)
}
