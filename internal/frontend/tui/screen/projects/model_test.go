package projects_test

import (
	"errors"
	"strings"
	"testing"
	"time"

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

// drainCmd executes a tea.Cmd (including batches) and feeds every returned
// message back into the model. Mirrors the worktime test helper pattern.
// Timer-based tea.Tick commands are skipped with a short deadline so tests
// don't stall.
func drainCmd(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msgCh := make(chan tea.Msg, 1)
	go func() {
		defer func() { _ = recover() }()
		msgCh <- cmd()
	}()
	var msg tea.Msg
	select {
	case msg = <-msgCh:
	case <-time.After(100 * time.Millisecond):
		// tea.Tick or other long-blocking cmd — drop it.
		return m
	}
	if msg == nil {
		return m
	}
	// tea.BatchMsg carries a slice of cmds — run each in turn.
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmd(t, m, c)
		}
		return m
	}
	updated, nextCmd := m.Update(msg)
	return drainCmd(t, updated, nextCmd)
}

func runUntilLoaded(t *testing.T, m projects.Model) tea.Model {
	t.Helper()
	cmd := m.Init()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return drainCmd(t, updated, cmd)
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
	// ANSI escapes interleave the letters — strip them before substring checks.
	out := ansi.Strip(updated.View().Content)
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "existing") {
		t.Errorf("View should list both projects, got:\n%s", out)
	}
	// After Task 18 the titlebox title is the tab-strip, not the count.
	// The tab labels should be visible in the title bar.
	if !strings.Contains(out, "Quellverzeichnisse") {
		t.Errorf("View should show tab strip with 'Quellverzeichnisse', got:\n%s", out)
	}
}

func TestInit_LoadError_DisplaysMessage(t *testing.T) {
	f := newFixture()
	f.scanner.err = errors.New("scan failed")
	updated := runUntilLoaded(t, f.model())
	if got := ansi.Strip(updated.View().Content); !strings.Contains(got, "scan failed") {
		t.Errorf("View should surface load error, got:\n%s", got)
	}
}

func TestEmpty_AfterLoad_ShowsHelp(t *testing.T) {
	f := newFixture()
	updated := runUntilLoaded(t, f.model())
	if got := ansi.Strip(updated.View().Content); !strings.Contains(got, "$SOURCECODE_ROOT prüfen") {
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
	// WithState accepts a tab-prefixed filter format (mirrors worktime).
	// Restoring "tab=quellverzeichnisse|foo" with cursor 5 should persist.
	restored := f.model().WithState("tab=quellverzeichnisse|foo", 5)
	m, ok := restored.(projects.Model)
	if !ok {
		t.Fatalf("WithState should return a projects.Model, got %T", restored)
	}
	// StateFilter encodes the active tab, so it contains "tab=…|foo".
	sf := m.StateFilter()
	if !strings.Contains(sf, "foo") {
		t.Errorf("StateFilter should contain restored filter 'foo', got %q", sf)
	}
	if !strings.HasPrefix(sf, "tab=") {
		t.Errorf("StateFilter should start with 'tab=', got %q", sf)
	}
	if m.StateCursor() != 5 {
		t.Errorf("StateCursor: got %d", m.StateCursor())
	}
}
