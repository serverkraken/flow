package projects_test

// Standalone-mode close-key tests for the projects screen (D2).
// Standalone = `flow projects` as a tmux-display-popup; embedded = sidekick.
// Only standalone gets q/esc → tea.Quit; embedded behavior must not change.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/usecase"
)

func standaloneModel(f *fixture) projects.Model {
	reader := &usecase.ProjectsReader{Scanner: f.scanner, Tmux: f.tmux}
	switcher := &usecase.ProjectSwitcher{Tmux: f.tmux}
	return projects.New(theme.Load(), "/Users/dev/Sourcecode", reader, switcher, projects.WithStandalone())
}

// TestStandaloneProjects_Q_Quits verifies that pressing 'q' in normal mode on
// the standalone projects popup produces tea.Quit.
func TestStandaloneProjects_Q_Quits(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := standaloneModel(f)
	updated := runUntilLoaded(t, m)
	_, cmd := updated.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("standalone projects: q in normal mode must return a cmd")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); !isQuit {
		t.Errorf("standalone projects: q must produce tea.QuitMsg, got %T", msg)
	}
}

// TestStandaloneProjects_Esc_NormalMode_Quits verifies that Esc in normal
// mode (filter not focused) on the standalone projects popup quits.
func TestStandaloneProjects_Esc_NormalMode_Quits(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := standaloneModel(f)
	updated := runUntilLoaded(t, m)
	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("standalone projects: esc in normal mode must return a cmd")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); !isQuit {
		t.Errorf("standalone projects: esc in normal mode must produce tea.QuitMsg, got %T", msg)
	}
}

// TestEmbeddedProjects_Q_DoesNotQuit verifies that embedded (sidekick) projects
// still routes 'q' to type-to-filter — embedded behavior must not change.
func TestEmbeddedProjects_Q_DoesNotQuit(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := f.model() // ModeEmbedded (default)
	updated := runUntilLoaded(t, m)
	_, cmd := updated.Update(tea.KeyPressMsg{Text: "q"})
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Errorf("embedded projects: q must NOT produce tea.QuitMsg")
		}
	}
}

// TestStandaloneProjects_Esc_FromFilterEmpty_Quits verifies that pressing esc
// in filter mode when the filter is empty quits the standalone popup (I1 fix).
// Previously two esc presses were required: first cleared/blurred; second quit.
func TestStandaloneProjects_Esc_FromFilterEmpty_Quits(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := standaloneModel(f)
	updated := runUntilLoaded(t, m)

	// Open filter with "/" — filter is now focused.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "/"})

	// Press esc with empty filter — must produce tea.Quit.
	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("standalone projects: esc from empty filter must return a cmd")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); !isQuit {
		t.Errorf("standalone projects: esc from empty filter must produce tea.QuitMsg, got %T", msg)
	}
}

// TestStandaloneProjects_Esc_FromFilterNonEmpty_ClearsFirst verifies that esc
// from a non-empty filter clears the filter without quitting (one-step clear).
func TestStandaloneProjects_Esc_FromFilterNonEmpty_ClearsFirst(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := standaloneModel(f)
	updated := runUntilLoaded(t, m)

	// Type "al" to populate the filter.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "a"})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "l"})

	// Press esc — should clear+blur, NOT quit.
	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Errorf("standalone projects: esc from non-empty filter must NOT quit, got QuitMsg")
		}
	}
}

// TestEmbeddedProjects_Esc_FromFilterEmpty_DoesNotQuit verifies that embedded
// mode does NOT quit when esc is pressed from an empty filter.
func TestEmbeddedProjects_Esc_FromFilterEmpty_DoesNotQuit(t *testing.T) {
	f := newFixture(domain.SourceDir{Name: "alpha", Path: "/p/alpha"})
	m := f.model() // ModeEmbedded (default)
	updated := runUntilLoaded(t, m)

	// Open filter with "/".
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "/"})

	// Press esc with empty filter — must NOT quit.
	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Errorf("embedded projects: esc from empty filter must NOT produce tea.QuitMsg")
		}
	}
}
