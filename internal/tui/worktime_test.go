package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestLoadedPopulatesAndViewRenders(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{
		sessions: []domain.WorkSession{run},
		projects: []domain.Project{{ID: "p1", Name: "Flow"}},
		now:      start.Add(25 * time.Minute),
	})
	m = next.(Model)
	if m.running == nil || m.running.ID != "s1" {
		t.Fatal("running session not detected from loaded sessions")
	}
	if !strings.Contains(m.View().Content, "00:25") {
		t.Fatalf("running elapsed not rendered:\n%s", m.View().Content)
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
}

func TestTickAdvancesNow(t *testing.T) {
	m := New(nil, "tester")
	t0 := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	next, _ := m.Update(tickMsg(t0))
	if got := next.(Model).now; !got.Equal(t0) {
		t.Fatalf("tick now = %v", got)
	}
}

func TestErrMsgSetsErr(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(errMsg{err: domain.ErrAlreadyRunning})
	if next.(Model).err == nil {
		t.Fatal("expected err to be set")
	}
}

func TestEventsReadyAndWaitForEvent(t *testing.T) {
	m := New(nil, "tester")
	ch := make(chan apiclient.ClientEvent, 1)
	ch <- apiclient.ClientEvent{Type: "session.started"}
	close(ch)

	next, cmd := m.Update(eventsReadyMsg{ch: ch})
	if next.(Model).events == nil {
		t.Fatal("events channel not stored")
	}
	if cmd == nil {
		t.Fatal("expected waitForEvent cmd")
	}

	// Drive the waitForEvent cmd — it reads from the closed channel.
	msg := cmd()
	if msg == nil {
		t.Fatal("expected event msg from cmd")
	}
	next2, _ := next.(Model).Update(msg)
	_ = next2
}

func TestInitReturnsBatch(t *testing.T) {
	m := New(nil, "tester")
	cmd := m.Init()
	// With nil client, reload/subscribe return nil — Init returns a batch
	// containing only the tick. It must not panic.
	_ = cmd
}

func TestSKeyStartsWhenIdle(t *testing.T) {
	// client is nil — should return nil cmd (guard branch)
	m := New(nil, "tester")
	next, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = next
	// nil client guard: cmd must be nil
	if cmd != nil {
		t.Fatal("expected nil cmd for nil client")
	}
}

func TestXKeyOpensBookingWhenRunning(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{sessions: []domain.WorkSession{run}, now: start})
	m = next.(Model)

	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	if !next2.(Model).booking {
		t.Fatal("x should open booking mode")
	}
}

func TestXKeyNoOpWhenIdle(t *testing.T) {
	m := New(nil, "tester")
	next, cmd := m.Update(tea.KeyPressMsg{Text: "x"})
	_ = next
	if cmd != nil {
		t.Fatal("x while idle should be no-op")
	}
}

func TestBookingEscCancels(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{sessions: []domain.WorkSession{run}, now: start})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = next2.(Model) // booking=true

	next3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next3.(Model).booking {
		t.Fatal("esc should cancel booking")
	}
}

func TestBookingTypingAndBackspace(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{sessions: []domain.WorkSession{run}, now: start})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = next2.(Model) // booking=true

	// Type "Flo"
	for _, ch := range "Flo" {
		next3, _ := m.Update(tea.KeyPressMsg{Text: string(ch)})
		m = next3.(Model)
	}
	if m.newName != "Flo" {
		t.Fatalf("expected 'Flo', got %q", m.newName)
	}
	// Backspace removes last char
	next4, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next4.(Model)
	if m.newName != "Fl" {
		t.Fatalf("expected 'Fl' after backspace, got %q", m.newName)
	}
}

func TestBookingJKNavigation(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{
		sessions: []domain.WorkSession{run},
		projects: []domain.Project{{ID: "p1", Name: "A"}, {ID: "p2", Name: "B"}},
		now:      start,
	})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = next2.(Model) // booking=true, sel=0

	// j moves selection down
	next3, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next3.(Model)
	if m.sel != 1 {
		t.Fatalf("j: sel = %d, want 1", m.sel)
	}
	// k moves selection up
	next4, _ := m.Update(tea.KeyPressMsg{Text: "k"})
	m = next4.(Model)
	if m.sel != 0 {
		t.Fatalf("k: sel = %d, want 0", m.sel)
	}
}

func TestBookingEnterNoProjectsKeepsBooking(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	// No projects loaded — hitting Enter should keep booking=true
	next, _ := m.Update(loadedMsg{sessions: []domain.WorkSession{run}, now: start})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = next2.(Model)

	next3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !next3.(Model).booking {
		t.Fatal("enter with no projects and no name should keep booking")
	}
}

func TestViewBookingRendersProjectList(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{
		sessions: []domain.WorkSession{run},
		projects: []domain.Project{{ID: "p1", Name: "Flow"}, {ID: "p2", Name: "Kompendium", Glyph: "K"}},
		now:      start.Add(5 * time.Minute),
	})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = next2.(Model) // booking=true

	out := m.View().Content
	if !strings.Contains(out, "Flow") {
		t.Fatalf("booking view missing project 'Flow':\n%s", out)
	}
}

func TestViewErrDisplayed(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(errMsg{err: domain.ErrAlreadyRunning})
	out := next.(Model).View().Content
	if !strings.Contains(out, "error:") {
		t.Fatalf("view missing error display:\n%s", out)
	}
}

func TestGlyphOr(t *testing.T) {
	if glyphOr("") != "●" {
		t.Fatal("empty glyph should return default")
	}
	if glyphOr("K") != "K" {
		t.Fatal("non-empty glyph should be returned")
	}
}

func TestFmtDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{90 * time.Minute, "01:30"},
		{0, "00:00"},
		{-time.Minute, "00:00"}, // negative clamped to 0
	}
	for _, tc := range cases {
		if got := fmtDur(tc.d); got != tc.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestUnhandledMsg(t *testing.T) {
	m := New(nil, "tester")
	// An unknown message type must be handled gracefully (no cmd returned).
	next, cmd := m.Update(struct{}{})
	_ = next
	if cmd != nil {
		t.Fatal("unexpected cmd for unhandled msg")
	}
}

// newFakeSrv returns a test HTTP server that handles session/project REST
// endpoints with minimal in-memory state, plus the apiclient pointed at it.
func newFakeSrv(t *testing.T) (*apiclient.Client, func()) {
	t.Helper()
	start := time.Now().UTC()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		ws, _ := domain.NewWorkSession("s1", "u1", nil, start)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ws)
	})
	mux.HandleFunc("POST /api/v1/sessions/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		stop := start.Add(30 * time.Minute)
		ws, _ := domain.NewWorkSession(r.PathValue("id"), "u1", nil, start)
		ws.Stop = &stop
		_ = json.NewEncoder(w).Encode(ws)
	})
	mux.HandleFunc("POST /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		p, _ := domain.NewProject("p1", "u1", "Flow", "flow", time.Now())
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	})
	srv := httptest.NewServer(mux)
	c := apiclient.New(srv.URL, "tok")
	return c, srv.Close
}

func TestStartCmdCallsAPI(t *testing.T) {
	c, stop := newFakeSrv(t)
	defer stop()
	m := New(c, "tester")
	cmd := m.startCmd()
	msg := cmd()
	// Successful call returns nil (not an errMsg).
	if _, ok := msg.(errMsg); ok {
		t.Fatalf("startCmd returned errMsg: %v", msg.(errMsg).err)
	}
}

func TestStopCmdCallsAPI(t *testing.T) {
	c, stop := newFakeSrv(t)
	defer stop()
	m := New(c, "tester")
	cmd := m.stopCmd("s1", "p1")
	msg := cmd()
	if _, ok := msg.(errMsg); ok {
		t.Fatalf("stopCmd returned errMsg: %v", msg.(errMsg).err)
	}
}

func TestCreateAndStopCmdCallsAPI(t *testing.T) {
	c, stop := newFakeSrv(t)
	defer stop()
	m := New(c, "tester")
	cmd := m.createAndStopCmd("s1", "NewProject")
	msg := cmd()
	if _, ok := msg.(errMsg); ok {
		t.Fatalf("createAndStopCmd returned errMsg: %v", msg.(errMsg).err)
	}
}

func TestWorktime_TodaySaldoRendered(t *testing.T) {
	m := New(nil, "tester")
	// LoggedMin:120 (2h), TargetMin:480 (8h), SaldoMin:-360 (negative → styleWarn)
	next, _ := m.Update(statsLoadedMsg{
		today: apiclient.Today{
			Date:      "2026-06-15",
			LoggedMin: 120,
			TargetMin: 480,
			SaldoMin:  -360,
		},
		burndown: apiclient.Burndown{
			TotalMin:  480,
			TargetMin: 960,
			SaldoMin:  -480,
		},
	})
	out := next.(Model).View().Content
	if !strings.Contains(out, "heute") {
		t.Fatalf("view missing 'heute' saldo line:\n%s", out)
	}
	if !strings.Contains(out, "2h 00m") {
		t.Fatalf("view missing logged '2h 00m':\n%s", out)
	}
	if !strings.Contains(out, "8h 00m") {
		t.Fatalf("view missing target '8h 00m':\n%s", out)
	}
	if !strings.Contains(out, "Monat") {
		t.Fatalf("view missing 'Monat' burndown line:\n%s", out)
	}
}

func TestWorktime_TodaySaldoPositive(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(statsLoadedMsg{
		today: apiclient.Today{
			LoggedMin: 510,
			TargetMin: 480,
			SaldoMin:  30,
		},
	})
	out := next.(Model).View().Content
	if !strings.Contains(out, "+0h 30m") {
		t.Fatalf("view missing positive saldo '+0h 30m':\n%s", out)
	}
}

func TestFmtMin(t *testing.T) {
	cases := []struct {
		min  int
		want string
	}{
		{0, "0h 00m"},
		{60, "1h 00m"},
		{90, "1h 30m"},
		{480, "8h 00m"},
		{-10, "0h 00m"}, // clamped to 0
	}
	for _, tc := range cases {
		if got := fmtMin(tc.min); got != tc.want {
			t.Errorf("fmtMin(%d) = %q, want %q", tc.min, got, tc.want)
		}
	}
}

func TestFmtSaldo(t *testing.T) {
	cases := []struct {
		min  int
		want string
	}{
		{0, "+0h 00m"},
		{30, "+0h 30m"},
		{90, "+1h 30m"},
		{-360, "-6h 00m"},
		{-90, "-1h 30m"},
	}
	for _, tc := range cases {
		if got := fmtSaldo(tc.min); got != tc.want {
			t.Errorf("fmtSaldo(%d) = %q, want %q", tc.min, got, tc.want)
		}
	}
}

func TestReloadWithRealClient(t *testing.T) {
	mux := http.NewServeMux()
	start := time.Now().UTC()
	mux.HandleFunc("GET /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		ws, _ := domain.NewWorkSession("s1", "u1", nil, start)
		_ = json.NewEncoder(w).Encode([]domain.WorkSession{ws})
	})
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Project{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	m := New(c, "tester")
	cmd := m.reload()
	msg := cmd()
	if _, ok := msg.(loadedMsg); !ok {
		t.Fatalf("reload returned %T, want loadedMsg", msg)
	}
}
