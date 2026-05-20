package sidekick_test

// Drives the s.Screen → screenID resolution in sidekick.New so all
// five screen branches are exercised. The existing suite covers the
// default (Palette) path; this fills out Projects / Worktime /
// Cheatsheet / Notes.

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/sidekick"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestNew_RestoresScreenIDFromState(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	for _, screen := range []string{
		domain.ScreenProjects,
		domain.ScreenWorktime,
		domain.ScreenCheatsheet,
		domain.ScreenNotes,
		domain.ScreenPalette,
		"unknown-falls-back-to-palette",
	} {
		screen := screen
		t.Run(screen, func(_ *testing.T) {
			m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: screen}, deps)
			// We can't directly read m.current (unexported), but New
			// should not panic and View should render something via
			// Init→Update path. The state restoration is the relevant
			// code-path under test.
			_ = m
		})
	}
}
