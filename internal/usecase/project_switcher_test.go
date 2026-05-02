package usecase_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestProjectSwitcher_Switch_CreateThenAttach(t *testing.T) {
	tmux := &testutil.FakeTmux{}
	s := &usecase.ProjectSwitcher{Tmux: tmux}
	if err := s.Switch(domain.Project{Name: "flow", Path: "/Users/me/Sourcecode/flow"}); err != nil {
		t.Fatal(err)
	}
	if len(tmux.New) != 1 || tmux.New[0] != "flow@/Users/me/Sourcecode/flow" {
		t.Errorf("expected new session at correct path, got %v", tmux.New)
	}
	if len(tmux.Switches) != 1 || tmux.Switches[0] != "flow" {
		t.Errorf("expected switch to flow, got %v", tmux.Switches)
	}
}

func TestProjectSwitcher_Switch_ExistingSessionSkipsCreate(t *testing.T) {
	tmux := &testutil.FakeTmux{Sessions: []string{"flow"}}
	s := &usecase.ProjectSwitcher{Tmux: tmux}
	if err := s.Switch(domain.Project{Name: "flow"}); err != nil {
		t.Fatal(err)
	}
	if len(tmux.New) != 0 {
		t.Errorf("should not create existing session, got %v", tmux.New)
	}
	if len(tmux.Switches) != 1 {
		t.Errorf("should still switch, got %v", tmux.Switches)
	}
}

func TestProjectSwitcher_Switch_EmptyNameFails(t *testing.T) {
	s := &usecase.ProjectSwitcher{Tmux: &testutil.FakeTmux{}}
	if err := s.Switch(domain.Project{Name: ""}); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestProjectSwitcher_Switch_NewSessionErrPropagates(t *testing.T) {
	tmux := &testutil.FakeTmux{NewSessionErr: errors.New("boom")}
	s := &usecase.ProjectSwitcher{Tmux: tmux}
	if err := s.Switch(domain.Project{Name: "x", Path: "/tmp/x"}); err == nil {
		t.Error("expected error")
	}
}

func TestProjectSwitcher_Switch_SwitchErrPropagates(t *testing.T) {
	tmux := &testutil.FakeTmux{Sessions: []string{"flow"}, SwitchErr: errors.New("boom")}
	s := &usecase.ProjectSwitcher{Tmux: tmux}
	if err := s.Switch(domain.Project{Name: "flow"}); err == nil {
		t.Error("expected error")
	}
}
