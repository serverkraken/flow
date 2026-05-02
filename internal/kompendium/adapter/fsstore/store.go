package fsstore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Store implements ports.NoteStore by reading and writing markdown files
// under a notebook root directory.
type Store struct {
	root string
}

// New returns a Store rooted at notebookRoot, creating the directory if it
// doesn't already exist.
func New(notebookRoot string) (*Store, error) {
	if err := os.MkdirAll(notebookRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create notebook root %q: %w", notebookRoot, err)
	}
	return &Store{root: notebookRoot}, nil
}

// Root reports the absolute notebook root.
func (s *Store) Root() string { return s.root }

// Path implements ports.NoteStore.
func (s *Store) Path(id domain.ID) string {
	return filepath.Join(s.root, filepath.FromSlash(id.Path()))
}

var _ ports.NoteStore = (*Store)(nil)
