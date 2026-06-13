package usecase

import (
	"errors"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ProjectSwitcher attaches the user's tmux client to a project. Creates
// the session in the project's directory if it doesn't exist yet, then
// switches.
type ProjectSwitcher struct {
	Tmux ports.Tmux
}

// Switch attaches to the source dir's tmux session. If no such session
// exists, a new one is created at p.Path before the switch.
func (s *ProjectSwitcher) Switch(p domain.SourceDir) error {
	if p.Name == "" {
		return errors.New("projektname darf nicht leer sein")
	}
	if !s.Tmux.HasSession(p.Name) {
		if err := s.Tmux.NewSessionAt(p.Name, p.Path); err != nil {
			return err
		}
	}
	return s.Tmux.SwitchClient(p.Name)
}
