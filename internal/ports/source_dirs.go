package ports

import "github.com/serverkraken/flow/internal/domain"

// SourceDirScanner enumerates the user's source directories. The adapter
// resolves the root location (env var $SOURCECODE_ROOT, defaulting to
// ~/Sourcecode) and filters to directories containing a .git entry.
//
// Each SourceDir carries Name + absolute Path; HasTmuxSession is left
// unset here and filled in by the use case via the Tmux port.
type SourceDirScanner interface {
	List() ([]domain.SourceDir, error)
}
