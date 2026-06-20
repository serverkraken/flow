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
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
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
func (f *fakeAPI) ListSessionsSince(context.Context, time.Time) ([]domain.WorkSession, error) {
	return f.sessions, nil
}
func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error) { return f.projects, nil }
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
	// Down moves cursor forward.
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if r2.(*TodayRoute).cursor != 1 {
		t.Fatalf("cursor Down = %d, want 1", r2.(*TodayRoute).cursor)
	}
	// Down again moves to 2.
	r3, _ := r2.(*TodayRoute).Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if r3.(*TodayRoute).cursor != 2 {
		t.Fatalf("cursor Down again = %d, want 2", r3.(*TodayRoute).cursor)
	}
	// Down at last item clamps (no wrap to 0).
	r4, _ := r3.(*TodayRoute).Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if r4.(*TodayRoute).cursor != 2 {
		t.Fatalf("cursor Down at bottom = %d, want 2 (no wrap)", r4.(*TodayRoute).cursor)
	}
}

func TestToday_ArrowsClampNoWrap(t *testing.T) {
	r := newTestRoute(&fakeAPI{})
	r.st = todayState{Completed: make([]completedSession, 3)}
	r.loaded = true

	// Up at index 0 must clamp (no wrap to last item).
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if r2.(*TodayRoute).cursor != 0 {
		t.Fatalf("Up at top must clamp (no wrap): cursor=%d, want 0", r2.(*TodayRoute).cursor)
	}

	// 'j' must no longer navigate.
	r3, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r3.(*TodayRoute).cursor != 0 {
		t.Fatalf("'j' must not move: cursor=%d, want 0", r3.(*TodayRoute).cursor)
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
	// feed the project list into the fuzzylist via projectsMsg
	r.booking.list = r.booking.list.SetItems(projectItems(f.projects))
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

func TestBooking_FuzzylistClampsOnShorterProjectList(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: true, ActiveID: "run"}
	r.dialog = dialogBooking
	r.booking = bookingState{list: fuzzylist.New(
		[]fuzzylist.Item{{ID: "p1", Label: "Alpha"}, {ID: "p2", Label: "Beta"}, {ID: "p3", Label: "Gamma"}},
		theme.Load(),
	).WithCreateHint("neu: %s")}
	// navigate to item 2 (cursor=2)
	for i := 0; i < 2; i++ {
		r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	// a refresh with a shorter list must clamp cursor (no panic on subsequent enter)
	r.Update(projectsMsg{projects: []domain.Project{{ID: "p1", Name: "Alpha"}}})
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
			r.booking = bookingState{list: fuzzylist.New([]fuzzylist.Item{{ID: "p1", Label: "Flow"}}, theme.Load()).WithCreateHint("neu: %s")}
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

func TestTodayRoute_capturesInputWhileDialogOpen(t *testing.T) {
	r := newTestRoute(&fakeAPI{})
	if r.CapturesInput() {
		t.Fatal("Today should not capture input in the list state")
	}
	// Open the booking dialog: press 's' while a session is running → dialogBooking.
	r.loaded = true
	r.st = todayState{Running: true, ActiveID: "run"}
	r.Update(keyPress("s"))
	if !r.CapturesInput() {
		t.Fatal("Today should capture input while a dialog is open")
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
