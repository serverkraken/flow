package domain

import (
	"bytes"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// NoteType classifies a note as a daily journal entry, a project-scoped note,
// or a free-form note.
type NoteType string

// Defined NoteType values.
const (
	TypeDaily   NoteType = "daily"
	TypeProject NoteType = "project"
	TypeFree    NoteType = "free"
)

// IsValid reports whether t is one of the defined NoteType values.
func (t NoteType) IsValid() bool {
	switch t {
	case TypeDaily, TypeProject, TypeFree:
		return true
	}
	return false
}

// Frontmatter is the YAML header of a note file.
type Frontmatter struct {
	ID      string   `yaml:"id"`
	Type    NoteType `yaml:"type"`
	Project string   `yaml:"project,omitempty"`
	Date    string   `yaml:"date,omitempty"`
	Title   string   `yaml:"title,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
	Daily   string   `yaml:"daily,omitempty"`
}

// Sentinel errors for frontmatter handling.
var (
	// ErrNoFrontmatter signals a file that does not start with a frontmatter block.
	ErrNoFrontmatter = errors.New("frontmatter not found")
	// ErrMalformedFrontmatter signals a frontmatter block with bad delimiters or YAML.
	ErrMalformedFrontmatter = errors.New("frontmatter malformed")
	// ErrInvalidFrontmatter signals a frontmatter block whose fields fail validation.
	ErrInvalidFrontmatter = errors.New("frontmatter invalid")
)

// Validate reports whether the frontmatter has a coherent set of fields.
func (fm Frontmatter) Validate() error {
	if fm.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidFrontmatter)
	}
	if !fm.Type.IsValid() {
		return fmt.Errorf("%w: type %q is not a valid NoteType", ErrInvalidFrontmatter, fm.Type)
	}
	if fm.Type == TypeProject && fm.Project == "" {
		return fmt.Errorf("%w: project type requires non-empty project field", ErrInvalidFrontmatter)
	}
	return nil
}

const (
	fmDelimNL = "---\n"
	fmDelim   = "---"
)

// HasFrontmatter reports whether content begins with a frontmatter opening
// delimiter.
func HasFrontmatter(content []byte) bool {
	return bytes.HasPrefix(content, []byte(fmDelimNL))
}

// ParseFrontmatter splits content into frontmatter and body. It expects content
// to start with "---\n", a YAML block, and a closing "---" on its own line.
// LF line endings only.
func ParseFrontmatter(content []byte) (Frontmatter, []byte, error) {
	rest, ok := stripOpeningDelim(content)
	if !ok {
		return Frontmatter{}, nil, ErrNoFrontmatter
	}
	yamlPart, body, ok := splitClosingDelim(rest)
	if !ok {
		return Frontmatter{}, nil, ErrMalformedFrontmatter
	}
	var fm Frontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return Frontmatter{}, nil, fmt.Errorf("%w: %v", ErrMalformedFrontmatter, err)
	}
	return fm, body, nil
}

func stripOpeningDelim(content []byte) ([]byte, bool) {
	if bytes.HasPrefix(content, []byte(fmDelimNL)) {
		return content[len(fmDelimNL):], true
	}
	return nil, false
}

func splitClosingDelim(rest []byte) (yamlPart, body []byte, ok bool) {
	if bytes.HasPrefix(rest, []byte(fmDelimNL)) {
		return nil, rest[len(fmDelimNL):], true
	}
	if bytes.Equal(rest, []byte(fmDelim)) {
		return nil, nil, true
	}
	if idx := bytes.Index(rest, []byte("\n"+fmDelimNL)); idx >= 0 {
		return rest[:idx], rest[idx+1+len(fmDelimNL):], true
	}
	if bytes.HasSuffix(rest, []byte("\n"+fmDelim)) {
		return rest[:len(rest)-1-len(fmDelim)], nil, true
	}
	return nil, nil, false
}

// Serialize emits the frontmatter as YAML between "---" delimiters, followed
// by body. The output uses LF line endings.
//
// yaml.Marshal cannot fail for the fixed Frontmatter shape (a flat struct
// without yaml.Marshaler implementations or cyclic references), so the error
// is dropped on purpose and the function returns []byte directly.
func (fm Frontmatter) Serialize(body []byte) []byte {
	encoded, _ := yaml.Marshal(fm) //nolint:errcheck // Frontmatter cannot fail to marshal — see doc comment.
	var buf bytes.Buffer
	buf.WriteString(fmDelimNL)
	buf.Write(encoded)
	buf.WriteString(fmDelimNL)
	buf.Write(body)
	return buf.Bytes()
}
