package fssourcedirs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// MaxDepth bounds the directory depth at which a `.git` entry still
// registers as a project. 5 matches the legacy `fd --max-depth 5`
// invocation flow used to ship.
const MaxDepth = 5

// Scanner enumerates project directories rooted at a fixed path.
type Scanner struct {
	root string
}

// New constructs a Scanner. root is typically resolved by the
// composition root from $SOURCECODE_ROOT with ~/Sourcecode as the
// fallback.
func New(root string) *Scanner {
	return &Scanner{root: filepath.Clean(root)}
}

// List walks the root and returns every directory containing a `.git`
// entry, sorted by relative name. Missing or unreadable root → empty
// slice with no error (first launch or env misconfig is tolerated).
//
// Subtrees that produce a walk error (typically EACCES) are skipped
// silently here — the projects screen prefers showing some projects
// over showing none. ListWithSkipped is the diagnostic-aware variant
// for callers that want to surface skipped paths to the user.
func (s *Scanner) List() ([]domain.SourceDir, error) {
	out, _, err := s.ListWithSkipped()
	return out, err
}

// ListWithSkipped behaves like List but also returns the paths whose
// subtrees were skipped due to walk errors (typically permission
// denied). Callers that want to surface "couldn't read foo/bar" to
// the user use this variant; the bare List swallows the diagnostic
// to keep the projects screen lossless on transient permission gaps.
func (s *Scanner) ListWithSkipped() ([]domain.SourceDir, []string, error) {
	info, err := os.Stat(s.root)
	if err != nil || !info.IsDir() {
		return nil, nil, nil
	}
	rootDepth := strings.Count(s.root, string(filepath.Separator))

	var rel, skipped []string
	walkErr := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable subtree: record the path so the caller can
			// surface a diagnostic, then skip it but keep walking
			// the rest of the tree.
			skipped = append(skipped, path)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		depth := strings.Count(path, string(filepath.Separator)) - rootDepth
		if depth > MaxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != ".git" {
			return nil
		}
		// .git can be a directory (regular repo) or a file (worktree).
		parent := filepath.Dir(path)
		if r, rerr := filepath.Rel(s.root, parent); rerr == nil && r != "" && r != "." {
			rel = append(rel, r)
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		return nil, skipped, walkErr
	}

	sort.Strings(rel)
	projects := make([]domain.SourceDir, 0, len(rel))
	for _, name := range rel {
		projects = append(projects, domain.SourceDir{
			Name: name,
			Path: filepath.Join(s.root, name),
		})
	}
	return projects, skipped, nil
}
