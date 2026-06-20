package daydetail_test

import (
	"context"
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
}

func (f *fakeAPI) ListSessionsRange(_ context.Context, since, until time.Time) ([]domain.WorkSession, error) {
	f.since, f.until = since, until
	return f.sessions, nil
}

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
