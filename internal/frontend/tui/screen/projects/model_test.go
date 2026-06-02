package projects_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeScanner struct {
	out []domain.SourceDir
	err error
}

func (f *fakeScanner) List() ([]domain.SourceDir, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.SourceDir, len(f.out))
	copy(out, f.out)
	return out, nil
}

type fixture struct {
	scanner *fakeScanner
	tmux    *testutil.FakeTmux
}

func newFixture(p ...domain.SourceDir) *fixture {
	return &fixture{
		scanner: &fakeScanner{out: append([]domain.SourceDir(nil), p...)},
		tmux:    &testutil.FakeTmux{Sessions: []string{"existing"}},
	}
}

func (f *fixture) model() projects.Model {
	reader := &usecase.ProjectsReader{Scanner: f.scanner, Tmux: f.tmux}
	switcher := &usecase.ProjectSwitcher{Tmux: f.tmux}
	return projects.New(theme.Load(), "/Users/dev/Sourcecode", reader, switcher)
}

func runUntilLoaded(t *testing.T, m projects.Model) tea.Model {
	t.Helper()
	cmd := m.Init()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(cmd())
	return updated
}

func TestNew_BeforeWindowSize_ViewIsEmpty(t *testing.T) {
	f := newFixture()
	if got := f.model().View().Content; got != "" {
		t.Errorf("View before WindowSizeMsg should be empty, got %q", got)
	}
}

func TestInit_LoadsAndAnnotatesSessions(t *testing.T) {
	f := newFixture(
		domain.SourceDir{Name: "alpha", Path: "/Users/dev/Sourcecode/alpha"},
		domain.SourceDir{Name: "existing", Path: "/Users/dev/Sourcecode/existing"},
	)
	updated := runUntilLoaded(t, f.model())
	// Row labels are rendered rune-by-rune (fuzzy-match emphasis), so the
	// ANSI escapes interleave the letters — strip them before substring
	// checks (mirror palette/model_test.go).
	out := ansi.Strip(updated.View().Content)
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "existing") {
		t.Errorf("View should list both projects, got:\n%s", out)
	}
	if !strings.Contains(out, "Sourcecode · 2") {
		t.Errorf("title should report '… · 2', got:\n%s", out)
	}
}

func TestInit_LoadError_DisplaysMessage(t *testing.T) {
	f := newFixture()
	f.scanner.err = errors.New("scan failed")
	updated := runUntilLoaded(t, f.model())
	if got := updated.View().Content; !strings.Contains(got, "scan failed") {
		t.Errorf("View should surface load error, got:\n%s", got)
	}
}

func TestEmpty_AfterLoad_ShowsHelp(t *testing.T) {
	f := newFixture()
	updated := runUntilLoaded(t, f.model())
	if got := updated.View().Content; !strings.Contains(got, "$SOURCECODE_ROOT prüfen") {
		t.Errorf("empty View should hint at $SOURCECODE_ROOT, got:\n%s", got)
	}
}

func TestEnter_SwitchesToProject(t *testing.T) {
	f := newFixture(
		domain.SourceDir{Name: "alpha", Path: "/Users/dev/Sourcecode/alpha"},
	)
	updated := runUntilLoaded(t, f.model())
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a tea.Cmd")
	}
	msg := cmd()
	if len(f.tmux.New) != 1 {
		t.Fatalf("expected 1 NewSessionAt call, got %d", len(f.tmux.New))
	}
	if f.tmux.New[0] != "alpha@/Users/dev/Sourcecode/alpha" {
		t.Errorf("NewSessionAt args: got %q", f.tmux.New[0])
	}
	if len(f.tmux.Switches) != 1 || f.tmux.Switches[0] != "alpha" {
		t.Errorf("SwitchClient calls: got %+v", f.tmux.Switches)
	}
	// switchedMsg → tea.Quit
	if _, ok := msg.(tea.QuitMsg); !ok {
		_, quit := updated.Update(msg)
		if quit == nil {
			t.Fatal("switchedMsg should propagate tea.Quit")
		}
	}
}

func TestSlashFiltersFuzzily(t *testing.T) {
	f := newFixture(
		domain.SourceDir{Name: "alpha-service", Path: "/x/alpha-service"},
		domain.SourceDir{Name: "beta", Path: "/x/beta"},
	)
	updated := runUntilLoaded(t, f.model())
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "/"})
	for _, r := range "alp" {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: string(r)})
	}
	out := ansi.Strip(updated.View().Content)
	if !strings.Contains(out, "alpha-service") {
		t.Errorf("filter should keep alpha-service, got:\n%s", out)
	}
	if strings.Contains(out, "beta") {
		t.Errorf("filter should hide beta, got:\n%s", out)
	}
}

func TestStateRoundtrip(t *testing.T) {
	f := newFixture()
	restored := f.model().WithState("foo", 5)
	m, ok := restored.(projects.Model)
	if !ok {
		t.Fatalf("WithState should return a projects.Model, got %T", restored)
	}
	if m.StateFilter() != "foo" {
		t.Errorf("StateFilter: got %q", m.StateFilter())
	}
	if m.StateCursor() != 5 {
		t.Errorf("StateCursor: got %d", m.StateCursor())
	}
}
