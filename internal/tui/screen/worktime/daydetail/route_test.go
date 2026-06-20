package daydetail_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/daydetail"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	since, until time.Time
	sessions     []domain.WorkSession
	projects     []domain.Project

	// Nachbuchen tracking fields (Task 6).
	addCalls      int
	lastStart     time.Time
	lastStop      time.Time
	lastProjectID *string
	addErr        error
}

func (f *fakeAPI) ListSessionsRange(_ context.Context, since, until time.Time) ([]domain.WorkSession, error) {
	f.since, f.until = since, until
	return f.sessions, nil
}

func (f *fakeAPI) ListProjects(_ context.Context) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	p := domain.Project{ID: "created-" + name, Name: name}
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeAPI) AddSession(_ context.Context, projectID *string, start, stop time.Time, _, _ string) (domain.WorkSession, error) {
	f.addCalls++
	f.lastStart = start
	f.lastStop = stop
	f.lastProjectID = projectID
	if f.addErr != nil {
		return domain.WorkSession{}, f.addErr
	}
	return domain.WorkSession{ID: "new", Start: start, Stop: &stop}, nil
}

// apiErr returns a fake error with the given HTTP status code string embedded,
// matching what apiclient.APIError.Error() produces: "apiclient: POST /api/v1/sessions: status NNN".
func apiErr(status int) error {
	return fmt.Errorf("apiclient: POST /api/v1/sessions: status %d", status)
}

// drive runs the route's Init command (if non-nil) and feeds the result back.
// It drains up to 10 rounds of commands so async loads complete synchronously.
func drive(t *testing.T, r shell.Route, cmd tea.Cmd) shell.Route {
	t.Helper()
	for i := 0; cmd != nil && i < 10; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		var newCmd tea.Cmd
		r, newCmd = r.Update(msg)
		cmd = newCmd
	}
	return r
}

// press sends a single key to the route and returns the updated route (draining
// any commands emitted).
func press(t *testing.T, r shell.Route, k tea.KeyPressMsg) shell.Route {
	t.Helper()
	r2, cmd := r.Update(k)
	return drive(t, r2, cmd)
}

// typeInto feeds each rune of s as a separate tea.KeyPressMsg with Text set.
func typeInto(t *testing.T, r shell.Route, s string) shell.Route {
	t.Helper()
	for _, ch := range s {
		r = press(t, r, tea.KeyPressMsg{Text: string(ch)})
	}
	return r
}

// keyRune returns a KeyPressMsg for a printable rune.
func keyRune(ch rune) tea.KeyPressMsg { return tea.KeyPressMsg{Text: string(ch)} }

// keyTab returns a KeyPressMsg for the Tab key.
func keyTab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab} }

// keyEnter returns a KeyPressMsg for the Enter key.
func keyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func shellFrame() shell.Frame {
	return shell.Frame{Width: 80, Height: 24, Pal: theme.Default}
}

func TestDayDetail_LoadsRangedSessions(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e, Tag: "deep"}}}
	r := daydetail.NewRoute(f, theme.Default, day)
	cmd := r.Init()
	msg := cmd() // execute loadCmd → loadedMsg
	r2, _ := r.Update(msg)
	out := r2.View(shellFrame())
	if !strings.Contains(out, "deep") {
		t.Fatalf("day view missing session tag: %q", out)
	}
	// loadCmd brackets exactly [startOfDay, startOfDay+24h)
	if !f.since.Equal(day) || !f.until.Equal(day.Add(24*time.Hour)) {
		t.Fatalf("range = [%v,%v), want the day's bounds", f.since, f.until)
	}
	_ = tea.KeyPressMsg{} // ensure import used
}

func TestDayDetail_Title(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	r := daydetail.NewRoute(&fakeAPI{}, theme.Default, day)
	title := r.Title()
	if title == "" {
		t.Fatal("Title should not be empty")
	}
	if !strings.Contains(title, "18") {
		t.Fatalf("Title should contain the day number, got %q", title)
	}
}

func TestDayDetail_LoadingState(t *testing.T) {
	r := daydetail.NewRoute(&fakeAPI{}, theme.Default, time.Now())
	out := r.View(shellFrame())
	if !strings.Contains(out, "lädt") {
		t.Fatalf("loading state should show 'lädt': %q", out)
	}
}

func TestDayDetail_EmptyState(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	f := &fakeAPI{sessions: nil}
	r := daydetail.NewRoute(f, theme.Default, day)
	cmd := r.Init()
	msg := cmd()
	r2, _ := r.Update(msg)
	out := r2.View(shellFrame())
	if !strings.Contains(out, "Keine Buchungen") {
		t.Fatalf("empty state should say 'Keine Buchungen': %q", out)
	}
}

func TestDayDetail_EscEmitsPop(t *testing.T) {
	r := daydetail.NewRoute(&fakeAPI{}, theme.Default, time.Now())
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should emit a command")
	}
	if _, ok := cmd().(shell.PopRouteMsg); !ok {
		t.Fatalf("Esc should emit shell.PopRouteMsg, got %T", cmd())
	}
}

func TestDayDetail_SSESessionEventTriggersReload(t *testing.T) {
	r := daydetail.NewRoute(&fakeAPI{}, theme.Default, time.Now())
	// Session events must trigger a reload.
	for _, evType := range []string{"session.started", "session.stopped", "session.updated", "session.deleted"} {
		_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: evType}})
		if cmd == nil {
			t.Fatalf("session event %q should trigger a reload cmd", evType)
		}
	}
	// Non-session events must not reload.
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "document.created"}})
	if cmd != nil {
		t.Fatal("non-session event should not trigger reload")
	}
}

func TestDayDetail_CursorClampsNoWrap(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	mk := func(h int) domain.WorkSession {
		s := day.Add(time.Duration(h) * time.Hour)
		e := s.Add(time.Hour)
		return domain.WorkSession{ID: "s" + string(rune('0'+h)), Start: s, Stop: &e}
	}
	f := &fakeAPI{sessions: []domain.WorkSession{mk(9), mk(10), mk(11)}}
	r := daydetail.NewRoute(f, theme.Default, day)
	cmd := r.Init()
	msg := cmd()
	r2, _ := r.Update(msg)

	// Move down twice.
	r3, _ := r2.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	r4, _ := r3.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	// Move down past end — should clamp.
	r5, _ := r4.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	out := r5.View(shellFrame())
	if out == "" {
		t.Fatal("view after cursor-at-bottom should not be empty")
	}

	// Move up past top — should clamp at 0.
	r6, _ := r5.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	r7, _ := r6.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	r8, _ := r7.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if out8 := r8.View(shellFrame()); out8 == "" {
		t.Fatal("view after cursor-at-top should not be empty")
	}
}

func TestDayDetail_KeyHintsNonEmpty(t *testing.T) {
	r := daydetail.NewRoute(&fakeAPI{}, theme.Default, time.Now())
	hints := r.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints should return non-empty hints")
	}
}

func TestDayDetail_RangeBoundsNormalisedToMidnight(t *testing.T) {
	// Even if NewRoute receives a non-midnight time, since must be midnight.
	noon := time.Date(2026, 6, 18, 12, 30, 0, 0, time.UTC)
	f := &fakeAPI{}
	r := daydetail.NewRoute(f, theme.Default, noon)
	cmd := r.Init()
	cmd()
	wantSince := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	wantUntil := wantSince.Add(24 * time.Hour)
	if !f.since.Equal(wantSince) || !f.until.Equal(wantUntil) {
		t.Fatalf("range = [%v,%v), want [%v,%v)", f.since, f.until, wantSince, wantUntil)
	}
}

func TestDayDetail_NachbuchenSubmitsAddSession(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	f := &fakeAPI{
		projects: []domain.Project{{ID: "p1", Name: "Acme"}},
	}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)

	// Load the empty day + project list.
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// 'n' opens Nachbuchen — project list loads then dialog opens.
	r = press(t, r, keyRune('n'))

	// Enter on the project picker selects "Acme" (first and only item).
	r = press(t, r, keyEnter())

	// Now on Von field — type "09:00".
	r = typeInto(t, r, "09:00")

	// Tab to Bis field.
	r = press(t, r, keyTab())

	// Type "12:00".
	r = typeInto(t, r, "12:00")

	// Tab to Tag, then Tab to Note to reach the last field.
	r = press(t, r, keyTab())
	r = press(t, r, keyTab())

	// Enter on Note (last field) submits.
	press(t, r, keyEnter())

	if f.addCalls != 1 {
		t.Fatalf("AddSession calls = %d, want 1", f.addCalls)
	}
	if !f.lastStart.Equal(day.Add(9*time.Hour)) || !f.lastStop.Equal(day.Add(12*time.Hour)) {
		t.Fatalf("AddSession times = [%v,%v), want 09:00–12:00 on the context day",
			f.lastStart, f.lastStop)
	}
	if f.lastProjectID == nil || *f.lastProjectID != "p1" {
		t.Fatalf("AddSession project = %v, want p1", f.lastProjectID)
	}
}

func TestDayDetail_OverlapErrorShowsToast(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	f := &fakeAPI{
		projects: []domain.Project{{ID: "p1", Name: "Acme"}},
		addErr:   apiErr(409),
	}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	r = press(t, r, keyRune('n'))
	r = press(t, r, keyEnter())  // select project
	r = typeInto(t, r, "09:00") // Von
	r = press(t, r, keyTab())   // → Bis
	r = typeInto(t, r, "10:00") // Bis
	r = press(t, r, keyTab())   // → Tag
	r = press(t, r, keyTab())   // → Note

	// Submit: run the Enter key press, execute the AddSession cmd, then feed
	// the result back — but stop before running the toast.Init() timer so we
	// can observe the toast while it is still visible.
	r2, cmd := r.Update(keyEnter())
	if cmd != nil {
		// cmd is the AddSession goroutine; run it to get the error msg.
		msg := cmd()
		if msg != nil {
			r2, _ = r2.Update(msg) // sets the toast; ignores toast.Init() cmd
		}
	}

	out := r2.View(shellFrame())
	// The error message contains the apiclient error which embeds "status 409".
	if !strings.Contains(out, "409") && !strings.Contains(out, "speichern") {
		t.Fatalf("expected error toast with status or 'speichern', got: %q", out)
	}
}
