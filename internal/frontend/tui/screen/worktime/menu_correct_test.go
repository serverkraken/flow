// White-box tests for the Korrektur-Flow.

package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// runningRig wires a worktime stack with an active session, so the
// Korrektur predicate passes and SessionWriter.CorrectStart has
// something to mutate.
type runningRig struct {
	deps     Deps
	clock    *testutil.FixedClock
	active   *testutil.FakeActiveSessionStore
	sessions *testutil.FakeSessionStore
}

func newRunningRig(t *testing.T) runningRig {
	t.Helper()
	clock := &testutil.FixedClock{T: time.Date(2026, 5, 6, 14, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{}
	active := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	lock := &testutil.FakeLock{}

	// Seed a running session: started at 09:30 today.
	start := time.Date(2026, 5, 6, 9, 30, 0, 0, time.Local)
	active.Active = &start

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: active, Targets: targets, Clock: clock}
	writer := &usecase.SessionWriter{Sessions: sessions, State: active, Lock: lock, Reader: reader, Clock: clock}

	return runningRig{
		deps: Deps{
			Reader:        reader,
			SessionWriter: writer,
			Clock:         clock,
		},
		clock:    clock,
		active:   active,
		sessions: sessions,
	}
}

func TestCorrectDefaultFor_PrefillsRunningStart(t *testing.T) {
	r := newRunningRig(t)
	if got := correctDefaultFor(r.deps); got != "09:30" {
		t.Errorf("default = %q, want 09:30", got)
	}
}

func TestCorrectDefaultFor_FallsBackToWallclock(t *testing.T) {
	clock := &testutil.FixedClock{T: time.Date(2026, 5, 6, 11, 45, 0, 0, time.Local)}
	deps := Deps{Clock: clock}
	if got := correctDefaultFor(deps); got != "11:45" {
		t.Errorf("default without Reader = %q, want 11:45 (wallclock)", got)
	}
}

func TestCorrectForm_EnterValidatesHHMM(t *testing.T) {
	r := newCorrectForm(pal(), "Startzeit korrigieren", "09:30")
	now := time.Date(2026, 5, 6, 14, 0, 0, 0, time.Local)
	_, _, ev := r.handleKey(keyName("enter"), now)
	if !ev.submitted {
		t.Fatal("Enter on valid HH:MM should submit")
	}
	want := time.Date(2026, 5, 6, 9, 30, 0, 0, time.Local)
	if !ev.parsed.Equal(want) {
		t.Errorf("parsed = %v, want %v", ev.parsed, want)
	}
}

func TestCorrectForm_RejectsEmptyInput(t *testing.T) {
	r := newCorrectForm(pal(), "X", "")
	r2, _, ev := r.handleKey(keyName("enter"), time.Now())
	if ev.submitted {
		t.Error("empty submit must NOT succeed")
	}
	if r2.errMsg == "" {
		t.Error("empty submit must populate errMsg")
	}
}

func TestCorrectForm_RejectsInvalidFormat(t *testing.T) {
	r := newCorrectForm(pal(), "X", "garbage")
	r2, _, ev := r.handleKey(keyName("enter"), time.Now())
	if ev.submitted {
		t.Error("invalid HH:MM must NOT submit")
	}
	if r2.errMsg == "" {
		t.Error("invalid HH:MM must populate errMsg")
	}
}

func TestCorrectForm_EscCancels(t *testing.T) {
	r := newCorrectForm(pal(), "X", "09:30")
	_, _, ev := r.handleKey(keyName("esc"), time.Now())
	if !ev.canceled {
		t.Error("Esc should cancel")
	}
}

func TestCorrectForm_ViewRendersInputAndHints(t *testing.T) {
	r := newCorrectForm(pal(), "Startzeit korrigieren", "09:30")
	out := r.view(pal(), 100)
	for _, want := range []string{"Startzeit korrigieren", "STARTZEIT", "09:30", "enter → speichern", "Esc → zurück"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q in:\n%s", want, out)
		}
	}
}

func TestCorrectCmd_DispatchesCorrectStart(t *testing.T) {
	r := newRunningRig(t)
	target := time.Date(2026, 5, 6, 9, 0, 0, 0, time.Local)
	cmd := correctCmd(r.deps, target)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("correct cmd err = %v", done.err)
	}
	// Active session start should now be 09:00, not the seeded 09:30.
	if r.active.Active == nil || !r.active.Active.Equal(target) {
		t.Errorf("active.Active = %v, want %v", r.active.Active, target)
	}
	if !strings.Contains(done.toast, "09:00") {
		t.Errorf("toast = %q, want it to mention 09:00", done.toast)
	}
}

func TestCorrectCmd_FailsWithoutSessionWriter(t *testing.T) {
	deps := Deps{}
	cmd := correctCmd(deps, time.Now())
	if cmd().(menuActionDoneMsg).err == nil {
		t.Error("correct without SessionWriter must fail cleanly")
	}
}

func TestCorrectCmd_NoActiveSessionPropagatesError(t *testing.T) {
	r := newRunningRig(t)
	r.active.Active = nil // session ended between predicate and dispatch
	cmd := correctCmd(r.deps, time.Now())
	done := cmd().(menuActionDoneMsg)
	if done.err == nil || !errorContains(done.err, "keine") {
		t.Errorf("err = %v, want error mentioning »keine« active session (domain.ErrNoActiveSession-shaped)", done.err)
	}
}

// — menu integration: Heute + IsRunning → Enter on Korrektur → form → submit —

func TestMenu_KorrekturVisibleOnHeuteWhileRunning(t *testing.T) {
	r := newRunningRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	var found bool
	for _, a := range m.filtered {
		if a.kind == menuActionCorrect {
			found = true
			break
		}
	}
	if !found {
		t.Error("Korrektur must show on Heute when a session is running")
	}
}

func TestMenu_KorrekturHiddenWhenIdle(t *testing.T) {
	r := newRunningRig(t)
	r.active.Active = nil
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for _, a := range m.filtered {
		if a.kind == menuActionCorrect {
			t.Error("Korrektur must NOT show when no session is running")
		}
	}
}

func TestMenu_KorrekturFlowEndsWithCorrectStart(t *testing.T) {
	r := newRunningRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionCorrect {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeCorrect {
		t.Fatalf("after Enter on Korrektur, subMode = %v, want correct", m.subMode)
	}
	// The form is pre-filled with 09:30; replace via backspace + new value.
	for i := 0; i < len("09:30"); i++ {
		m, _ = m.handleKey(keyName("backspace"))
	}
	for _, ch := range "09:00" {
		m, _ = m.handleKey(runeKey(ch))
	}
	m, cmd := m.handleKey(keyName("enter"))
	if cmd == nil {
		t.Fatal("submit should return a tea.Cmd")
	}
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("dispatch err = %v", done.err)
	}
	want := time.Date(2026, 5, 6, 9, 0, 0, 0, time.Local)
	if r.active.Active == nil || !r.active.Active.Equal(want) {
		t.Errorf("active start = %v, want %v", r.active.Active, want)
	}
	if m.subMode != menuSubModeList {
		t.Errorf("after dispatch, subMode = %v, want list", m.subMode)
	}
}

// Compile-time guard: domain.ParseHM stays in the public API of the
// package so menu_correct.go's import doesn't drift.
var _ = domain.ParseHM

func errorContains(err error, sub string) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), strings.ToLower(sub))
}
