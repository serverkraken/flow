package worktime

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

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

func newTestRoute(f *fakeAPI) *TodayRoute { return NewTodayRoute(f, fixedNow, theme.Load()) }

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
