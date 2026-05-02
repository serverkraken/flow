package ports

import "github.com/serverkraken/flow/internal/domain"

// ProjectScanner enumerates the user's project directories. The adapter
// resolves the root location (env var $SOURCECODE_ROOT, defaulting to
// ~/Sourcecode) and filters to directories containing a .git entry.
//
// Each Project carries Name + absolute Path; HasTmuxSession is left
// unset here and filled in by the use case via the Tmux port.
type ProjectScanner interface {
	List() ([]domain.Project, error)
}
