package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ProjectsReader enumerates the user's source directories, annotated
// with which ones currently have a tmux session attached.
type ProjectsReader struct {
	Scanner ports.SourceDirScanner
	Tmux    ports.Tmux
}

// List returns the source-dir list. tmux-session lookup failures are
// tolerated — the row is still returned, just without the session marker.
func (r *ProjectsReader) List() ([]domain.SourceDir, error) {
	projects, err := r.Scanner.List()
	if err != nil {
		return nil, err
	}
	sessionSet := map[string]bool{}
	if sessions, err := r.Tmux.ListSessions(); err == nil {
		for _, s := range sessions {
			sessionSet[s] = true
		}
	}
	for i := range projects {
		projects[i].HasTmuxSession = sessionSet[projects[i].Name]
	}
	return projects, nil
}
