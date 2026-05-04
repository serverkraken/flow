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
//
// Extra captures any user-added top-level keys (mood, weather, plugin
// metadata, aliases, …) so a Get→Put round-trip preserves them rather
// than silently dropping anything not in the closed struct. yaml.v3's
// ",inline" tag merges unknown keys into Extra on Unmarshal and splices
// them back at the top level on Marshal.
type Frontmatter struct {
	ID      string         `yaml:"id"`
	Type    NoteType       `yaml:"type"`
	Project string         `yaml:"project,omitempty"`
	Date    string         `yaml:"date,omitempty"`
	Title   string         `yaml:"title,omitempty"`
	Tags    []string       `yaml:"tags,omitempty"`
	Extra   map[string]any `yaml:",inline"`
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

// utf8BOM is the UTF-8 byte-order mark some editors prepend on save.
// It carries no semantic meaning but breaks the literal "---\n" prefix
// check below, so HasFrontmatter / ParseFrontmatter strip it before
// inspecting content.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// HasFrontmatter reports whether content begins with a frontmatter opening
// delimiter, tolerating a UTF-8 BOM and CRLF line endings as written by
// Windows or non-LF-defaulting editors.
func HasFrontmatter(content []byte) bool {
	content = stripBOM(content)
	return bytes.HasPrefix(content, []byte(fmDelimNL)) ||
		bytes.HasPrefix(content, []byte("---\r\n"))
}

// ParseFrontmatter splits content into frontmatter and body. It expects content
// to start with "---\n", a YAML block, and a closing "---" on its own line.
//
// Tolerates a leading UTF-8 BOM (some editors save with one) and CRLF
// line endings (Windows-created notes, copy-paste from web editors).
// Both are normalised to LF before parsing so the YAML block, body, and
// downstream consumers all see consistent line endings — without this,
// the "---\n" prefix check failed silently and reported ErrNoFrontmatter
// for files that clearly had frontmatter.
func ParseFrontmatter(content []byte) (Frontmatter, []byte, error) {
	content = normaliseLineEndings(stripBOM(content))
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

func stripBOM(content []byte) []byte {
	if bytes.HasPrefix(content, utf8BOM) {
		return content[len(utf8BOM):]
	}
	return content
}

func normaliseLineEndings(content []byte) []byte {
	if !bytes.Contains(content, []byte("\r")) {
		return content
	}
	// Handle CRLF first so a stray CR mid-document doesn't dupe the LF.
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
	return content
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
