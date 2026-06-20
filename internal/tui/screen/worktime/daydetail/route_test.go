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

	// Edit/Delete tracking fields (Task 7).
	editCalls    int
	lastEditID   string
	lastEditStop *time.Time
	editErr      error
	delCalls     int
	lastDelID    string
	delErr       error
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

func (f *fakeAPI) EditSession(_ context.Context, id string, _ *string, _, _ string, _ time.Time, stop *time.Time) (domain.WorkSession, error) {
	f.editCalls++
	f.lastEditID = id
	f.lastEditStop = stop
	if f.editErr != nil {
		return domain.WorkSession{}, f.editErr
	}
	return domain.WorkSession{ID: id}, nil
}

func (f *fakeAPI) DeleteSession(_ context.Context, id string) error {
	f.delCalls++
	f.lastDelID = id
	return f.delErr
}

// apiErr returns a fake error with the given HTTP status code string embedded,
// matching what apiclient.APIError.Error() produces: "apiclient: POST /api/v1/sessions: status NNN".
func apiErr(status int) error {
	return fmt.Errorf("apiclient: POST /api/v1/sessions: status %d", status)
}

// runCmd executes a tea.Cmd but gives up if it does not return promptly. Async
// I/O cmds (the fake API loads) complete instantly, while timer/tick cmds
// (toast's 2s tea.Tick, the textinput cursor blink at 530ms) would otherwise
// block the synchronous test driver and make every dialog test take seconds.
// Returning (nil,false) on timeout means "this is a non-terminal timer cmd —
// stop draining" without weakening any assertion.
func runCmd(cmd tea.Cmd) (tea.Msg, bool) {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg, true
	case <-time.After(10 * time.Millisecond):
		return nil, false
	}
}

// drive runs the route's Init command (if non-nil) and feeds the result back.
// It drains up to 10 rounds of commands so async loads complete synchronously,
// stopping at terminal messages (and skipping blocking timer/tick cmds).
func drive(t *testing.T, r shell.Route, cmd tea.Cmd) shell.Route {
	t.Helper()
	for i := 0; cmd != nil && i < 10; i++ {
		msg, ok := runCmd(cmd)
		if !ok || msg == nil {
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
	r = press(t, r, keyEnter())

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

	// After successful submit, the dialog must close.
	dr, ok := r.(*daydetail.Route)
	if !ok {
		t.Fatalf("route is %T, want *daydetail.Route", r)
	}
	if dr.DialogOpen() {
		t.Fatal("Nachbuchen dialog must close after successful AddSession")
	}
}

// clearField sends Backspace enough times to empty a prefilled HH:MM input.
func clearField(t *testing.T, r shell.Route) shell.Route {
	t.Helper()
	for i := 0; i < 8; i++ {
		r = press(t, r, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	return r
}

func TestDayDetail_EditSubmitsEditSession(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e, Tag: "old"}}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// 'e' opens the edit dialog on the selected row (row 0), prefilled 09:00/11:00.
	r = press(t, r, keyRune('e'))
	// Tab from Von → Bis, clear "11:00", type "12:00".
	r = press(t, r, keyTab())
	r = clearField(t, r)
	r = typeInto(t, r, "12:00")
	// Tab to Tag, Tab to Notiz (last field), then Enter to submit.
	r = press(t, r, keyTab())
	r = press(t, r, keyTab())
	r = press(t, r, keyEnter())

	if f.editCalls != 1 || f.lastEditID != "a" {
		t.Fatalf("EditSession calls=%d id=%q, want 1/a", f.editCalls, f.lastEditID)
	}
	if f.lastEditStop == nil || !f.lastEditStop.Equal(day.Add(12*time.Hour)) {
		t.Fatalf("edit stop = %v, want 12:00 on the context day", f.lastEditStop)
	}

	// After a successful submit the dialog must close.
	dr := r.(*daydetail.Route)
	if dr.DialogOpen() {
		t.Fatal("edit dialog must close after successful EditSession")
	}
}

func TestDayDetail_EditErrorKeepsDialogOpen(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{
		sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e, Tag: "old"}},
		editErr:  apiErr(409),
	}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	r = press(t, r, keyRune('e'))
	r = press(t, r, keyTab()) // → Bis
	r = press(t, r, keyTab()) // → Tag
	r = press(t, r, keyTab()) // → Notiz

	// Submit: run the edit cmd, feed the error msg back, but stop before the
	// toast timer so the dialog state is observable.
	r2, cmd := r.Update(keyEnter())
	if cmd != nil {
		if msg := cmd(); msg != nil {
			r2, _ = r2.Update(msg)
		}
	}
	dr := r2.(*daydetail.Route)
	if !dr.DialogOpen() {
		t.Fatal("edit dialog must stay open after an EditSession error (user must not lose input)")
	}
}

func TestDayDetail_DeleteConfirms(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e}}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	r = press(t, r, keyRune('d')) // open delete confirm
	_ = press(t, r, keyRune('y')) // confirm (confirm.Model accepts y/Enter)

	if f.delCalls != 1 || f.lastDelID != "a" {
		t.Fatalf("DeleteSession calls=%d id=%q, want 1/a", f.delCalls, f.lastDelID)
	}
}

func TestDayDetail_DeleteCancelDoesNotDelete(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e}}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	r = press(t, r, keyRune('d')) // open delete confirm
	r = press(t, r, keyRune('n')) // cancel

	if f.delCalls != 0 {
		t.Fatalf("DeleteSession calls=%d, want 0 after cancel", f.delCalls)
	}
	dr := r.(*daydetail.Route)
	if dr.DialogOpen() {
		t.Fatal("delete confirm must close after cancel")
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

	// CRITICAL: the dialog must stay OPEN on error so the user keeps the typed
	// project + Von + Bis + tag + note instead of having to re-enter everything.
	dr, ok := r2.(*daydetail.Route)
	if !ok {
		t.Fatalf("route is %T, want *daydetail.Route", r2)
	}
	if !dr.DialogOpen() {
		t.Fatal("Nachbuchen dialog must stay open after an AddSession error (user must not lose input)")
	}
	// And the populated fields must still be rendered (project name + Von value).
	if !strings.Contains(out, "Acme") {
		t.Fatalf("dialog should still render the picked project name 'Acme', got: %q", out)
	}
	if !strings.Contains(out, "09:00") {
		t.Fatalf("dialog should still render the typed Von value '09:00', got: %q", out)
	}
}
