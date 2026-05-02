package usecase_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestProjectsReader_AnnotatesActiveSessions(t *testing.T) {
	r := &usecase.ProjectsReader{
		Scanner: &testutil.FakeProjectScanner{Names: []string{"flow", "kompendium", "dotfiles"}},
		Tmux:    &testutil.FakeTmux{Sessions: []string{"flow", "dotfiles"}},
	}
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"flow":       true,
		"kompendium": false,
		"dotfiles":   true,
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(got))
	}
	for _, p := range got {
		if want[p.Name] != p.HasTmuxSession {
			t.Errorf("project %q: HasTmuxSession=%v, want %v", p.Name, p.HasTmuxSession, want[p.Name])
		}
	}
}

func TestProjectsReader_TmuxErrorIsTolerated(t *testing.T) {
	r := &usecase.ProjectsReader{
		Scanner: &testutil.FakeProjectScanner{Names: []string{"flow"}},
		Tmux:    &testutil.FakeTmux{ListSessionsErr: errors.New("tmux not running")},
	}
	got, err := r.List()
	if err != nil {
		t.Fatalf("tmux error should not propagate, got %v", err)
	}
	if len(got) != 1 || got[0].HasTmuxSession {
		t.Errorf("got %+v, expected single project without session marker", got)
	}
}

func TestProjectsReader_ScannerErrorPropagates(t *testing.T) {
	r := &usecase.ProjectsReader{
		Scanner: &testutil.FakeProjectScanner{Err: errors.New("perm denied")},
		Tmux:    &testutil.FakeTmux{},
	}
	if _, err := r.List(); err == nil {
		t.Error("expected scanner error to propagate")
	}
}
