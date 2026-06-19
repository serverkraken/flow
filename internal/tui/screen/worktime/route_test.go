package worktime

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
)

func keyPress(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: s} }
func keyEnterMsg() tea.KeyPressMsg       { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func confirmResult(ok bool) tea.Msg      { return confirm.ResultMsg{Confirmed: ok} }

type fakeAPI struct {
	today    apiclient.Today
	sessions []domain.WorkSession
	projects []domain.Project
	started  bool
	stopped  [2]string
	edited   string
	deleted  string
}

func (f *fakeAPI) GetToday(context.Context) (apiclient.Today, error)          { return f.today, nil }
func (f *fakeAPI) ListSessions(context.Context) ([]domain.WorkSession, error) { return f.sessions, nil }
func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error)     { return f.projects, nil }
func (f *fakeAPI) StartSession(context.Context, *string, string, string) (domain.WorkSession, error) {
	f.started = true
	return domain.WorkSession{ID: "new"}, nil
}
func (f *fakeAPI) StopSession(_ context.Context, id, pid string) (domain.WorkSession, error) {
	f.stopped = [2]string{id, pid}
	return domain.WorkSession{ID: id}, nil
}
func (f *fakeAPI) EditSession(_ context.Context, id string, _ *string, _, _ string, _ time.Time, _ *time.Time) (domain.WorkSession, error) {
	f.edited = id
	return domain.WorkSession{ID: id}, nil
}
func (f *fakeAPI) DeleteSession(_ context.Context, id string) error { f.deleted = id; return nil }
func (f *fakeAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	return domain.Project{ID: "p-" + name, Name: name}, nil
}

func fixedNow() time.Time { return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC) }

func newTestRoute(f *fakeAPI) *TodayRoute {
	return NewTodayRoute(f, fixedNow, theme.Load(), BuildRegistry(nil, theme.Load()))
}

func TestTodayRoute_wKeyEmitsSwitch(t *testing.T) {
	r := newTestRoute(&fakeAPI{})
	_, cmd := r.Update(keyPress("w"))
	if cmd == nil {
		t.Fatal("Today: w should emit a switch cmd")
	}
}

func TestRoute_LoadPopulatesState(t *testing.T) {
	f := &fakeAPI{today: apiclient.Today{TargetMin: 480, LoggedMin: 60}, sessions: []domain.WorkSession{
		{ID: "s1", Start: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC), Stop: ptr(time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC))},
	}}
	r := newTestRoute(f)
	cmd := r.Init()
	msg := cmd()
	r2, _ := r.Update(msg)
	rt := r2.(*TodayRoute)
	if len(rt.st.Completed) != 1 || rt.st.Target != 8*time.Hour {
		t.Fatalf("state not loaded: %+v", rt.st)
	}
}

func TestRoute_SSETriggersReload(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "session.started"}})
	if cmd == nil {
		t.Fatal("session.* event should trigger a reload cmd")
	}
	if _, c := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "document.created"}}); c != nil {
		t.Fatal("unrelated event should not reload")
	}
}

func TestRoute_NoDoubleTicker(t *testing.T) {
	active := time.Date(2026, 6, 14, 11, 0, 0, 0, time.UTC)
	f := &fakeAPI{today: apiclient.Today{Running: true}, sessions: []domain.WorkSession{{ID: "run", Start: active}}}
	r := newTestRoute(f)
	// first load arms the ticker
	_, c1 := r.Update(loadedMsg{today: f.today, sessions: f.sessions})
	if c1 == nil || !r.ticking {
		t.Fatalf("first load should arm ticker: cmd=%v ticking=%v", c1 != nil, r.ticking)
	}
	// a reload while already ticking must NOT arm a second ticker
	_, c2 := r.Update(loadedMsg{today: f.today, sessions: f.sessions})
	if c2 != nil {
		t.Fatal("second load while running must not arm a second ticker")
	}
}

func TestRoute_CursorMoves(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.st = todayState{Completed: make([]completedSession, 3)}
	r.applyKey("j")
	if r.cursor != 1 {
		t.Fatalf("cursor j = %d", r.cursor)
	}
	r.applyKey("k")
	r.applyKey("k")
	if r.cursor != 2 {
		t.Fatalf("cursor wrap = %d", r.cursor)
	}
}

func TestActions_StartWhenIdle(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: false}
	_, cmd := r.handleKey(keyPress("s"))
	if cmd == nil {
		t.Fatal("expected start cmd")
	}
	cmd()
	if !f.started {
		t.Fatal("StartSession not called")
	}
}

func TestActions_StopOpensBookingThenBooks(t *testing.T) {
	f := &fakeAPI{projects: []domain.Project{{ID: "p1", Name: "Flow"}}}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: true, ActiveID: "run"}
	_, _ = r.handleKey(keyPress("s"))
	if r.dialog != dialogBooking {
		t.Fatalf("dialog = %v, want booking", r.dialog)
	}
	r.booking.projects = f.projects
	_, cmd := r.handleKey(keyEnterMsg())
	if cmd != nil {
		cmd()
	}
	if r.dialog != dialogNone || f.stopped[1] != "p1" {
		t.Fatalf("stop booking failed: dialog=%v stopped=%v", r.dialog, f.stopped)
	}
}

func TestActions_DeleteConfirmCallsDelete(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Completed: []completedSession{{ID: "s1"}}}
	r.cursor = 0
	_, _ = r.handleKey(keyPress("D"))
	if r.dialog != dialogDelete {
		t.Fatalf("dialog = %v, want delete", r.dialog)
	}
	_, cmd := r.Update(confirmResult(true))
	if cmd != nil {
		cmd()
	}
	if f.deleted != "s1" {
		t.Fatalf("DeleteSession not called: %q", f.deleted)
	}
}

func TestBooking_SelClampedOnShorterProjectList(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: true, ActiveID: "run"}
	r.dialog = dialogBooking
	r.booking = bookingState{projects: []domain.Project{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}, sel: 2}
	// a refresh with a shorter list must clamp sel (no panic on subsequent enter)
	r.Update(projectsMsg{projects: []domain.Project{{ID: "p1"}}})
	if r.booking.sel != 0 {
		t.Fatalf("sel not clamped: %d", r.booking.sel)
	}
	// enter now books p1 without panicking
	_, cmd := r.handleKey(keyEnterMsg())
	if cmd != nil {
		cmd()
	}
	if f.stopped[1] != "p1" {
		t.Fatalf("expected stop on p1, got %q", f.stopped[1])
	}
}

func TestOpenEdit_GuardsCursorBounds(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Completed: []completedSession{{ID: "s1"}}}
	r.cursor = 5 // stale, beyond list
	if _, _ = r.openEdit(); r.dialog == dialogEdit {
		t.Fatal("openEdit should not open with out-of-range cursor")
	}
	if _, _ = r.openDelete(); r.dialog == dialogDelete {
		t.Fatal("openDelete should not open with out-of-range cursor")
	}
}

func TestView_RendersEachDialogAndHints(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	frame := shell.Frame{Width: 60, Height: 20, Pal: theme.Load()}

	if r.Title() == "" {
		t.Fatal("Title empty")
	}

	// not loaded → lädt placeholder
	if got := r.View(frame); got == "" {
		t.Fatal("View (unloaded) empty")
	}

	r.loaded = true
	r.st = todayState{
		Running:   true,
		ActiveID:  "run",
		Completed: []completedSession{{ID: "s1"}},
	}

	// body view + idle/running KeyHints
	if got := r.View(frame); got == "" {
		t.Fatal("View (body) empty")
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("KeyHints (body) empty")
	}
	r.st.Running = false
	if len(r.KeyHints()) == 0 {
		t.Fatal("KeyHints (idle) empty")
	}

	// each dialog branch must render via renderDialog + emit dialogHints
	for _, d := range []dialogKind{dialogBooking, dialogEdit, dialogDelete} {
		r.dialog = d
		if d == dialogBooking {
			r.booking = bookingState{projects: []domain.Project{{ID: "p1", Name: "Flow"}}}
		}
		if d == dialogEdit {
			_, _ = r.openEdit()
			r.dialog = dialogEdit
		}
		if got := r.View(frame); d != dialogDelete && got == "" {
			t.Fatalf("View dialog %v empty", d)
		}
		if len(r.KeyHints()) == 0 {
			t.Fatalf("dialogHints %v empty", d)
		}
	}
}

func TestActions_EditSubmitCallsEdit(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r.st = todayState{Completed: []completedSession{{ID: "s1", Start: start, Stop: stop}}}
	r.cursor = 0
	_, _ = r.handleKey(keyPress("E"))
	if r.dialog != dialogEdit {
		t.Fatalf("dialog = %v, want edit", r.dialog)
	}
	cmd := r.submitEdit()
	if cmd != nil {
		cmd()
	}
	if f.edited != "s1" {
		t.Fatalf("EditSession not called: %q", f.edited)
	}
}
