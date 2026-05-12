package usecase_test

// Lifecycle tests for SessionWriter: Start / Stop / Pause / Resume /
// Toggle / CorrectStart plus the StopBeforeStart sentinel guard.
// Manual-edit (AddManual / Edit / Delete / SetTag / SetNote) lives in
// session_writer_manual_test.go; error-propagation cases in
// session_writer_errors_test.go. Shared helpers (mkWriter, flaky
// stores) stay in session_writer_test.go.

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSessionWriter_Start_HappyPath(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	w := mkWriter(now)
	if err := w.Start(now); err != nil {
		t.Fatal(err)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Active == nil || !state.Active.Equal(now) {
		t.Errorf("active = %v, want %v", state.Active, now)
	}
}

func TestSessionWriter_Start_AlreadyRunning(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	prior := now.Add(-time.Hour)
	w := mkWriter(now, withActive(prior))
	err := w.Start(now)
	if !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestSessionWriter_StartForce_OverridesAndClearsPause(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	prior := now.Add(-time.Hour)
	pause := now.Add(-30 * time.Minute)
	w := mkWriter(now, func(r *usecase.WorktimeReader) {
		r.State = &testutil.FakeActiveSessionStore{Active: &prior, Pause: &pause}
	})
	w.State = w.Reader.State
	if err := w.StartForce(now); err != nil {
		t.Fatal(err)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if !state.Active.Equal(now) {
		t.Errorf("active should be overwritten, got %v", state.Active)
	}
	if state.Pause != nil {
		t.Errorf("pause should be cleared, got %v", state.Pause)
	}
}

func TestSessionWriter_Stop_LogsAndClears(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(8 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	s, err := w.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if s.Elapsed != 8*time.Hour {
		t.Errorf("Elapsed = %v, want 8h", s.Elapsed)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 {
		t.Errorf("expected 1 logged session, got %d", len(store.Sessions))
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Active != nil {
		t.Error("active should be cleared")
	}
}

func TestSessionWriter_Stop_CrossingMidnightSplits(t *testing.T) {
	start := time.Date(2026, 4, 29, 22, 0, 0, 0, time.Local)
	now := time.Date(2026, 4, 30, 1, 0, 0, 0, time.Local)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	s, err := w.Stop()
	if err != nil {
		t.Fatal(err)
	}
	// Returned Session has the original 3-hour span.
	if s.Elapsed != 3*time.Hour {
		t.Errorf("returned Elapsed = %v, want 3h", s.Elapsed)
	}
	// Date is anchored on the *start* day, not the stop day. A caller
	// printing s.Date for a session that began at 22:00 yesterday and
	// stopped at 01:00 today expects "yesterday" — anchoring on stop
	// would silently misattribute the session to today.
	wantDate := time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local)
	if !s.Date.Equal(wantDate) {
		t.Errorf("returned Date = %s, want %s (start day)", s.Date, wantDate)
	}
	// Stored sessions split into two rows.
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 2 {
		t.Fatalf("expected 2 logged rows, got %d", len(store.Sessions))
	}
}

func TestSessionWriter_Stop_NoActiveSession(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	w := mkWriter(now)
	_, err := w.Stop()
	if !errors.Is(err, domain.ErrNoActiveSession) {
		t.Errorf("expected ErrNoActiveSession, got %v", err)
	}
}

func TestSessionWriter_Stop_StopBeforeStartFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	future := now.Add(time.Hour)
	w := mkWriter(now, withActive(future))
	w.State = w.Reader.State
	_, err := w.Stop()
	if err == nil {
		t.Fatal("expected error for stop before start")
	}
	// Review finding Q5: the four call sites that previously raised
	// `errors.New("stoppzeit muss nach Startzeit liegen")` now share
	// one sentinel so callers can branch on errors.Is.
	if !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Errorf("got %v, want wrap of ErrStopBeforeStart", err)
	}
}

// TestSessionWriter_StopBeforeStart_SentinelAcrossCallSites guards
// review finding Q5 across the entry points that surface the sentinel
// (Stop, Toggle, Edit, AddManual) so future renames keep the contract.
// Pause has a separate idempotency test
// (TestSessionWriter_Pause_StopBeforeStart_IsIdempotent) — it swallows
// the sentinel by design, matching its ErrNoActiveSession contract.
func TestSessionWriter_StopBeforeStart_SentinelAcrossCallSites(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	future := now.Add(time.Hour)

	t.Run("Stop", func(t *testing.T) {
		w := mkWriter(now, withActive(future))
		w.State = w.Reader.State
		_, err := w.Stop()
		if !errors.Is(err, domain.ErrStopBeforeStart) {
			t.Errorf("Stop: got %v, want ErrStopBeforeStart", err)
		}
	})
	t.Run("Toggle", func(t *testing.T) {
		w := mkWriter(now, withActive(future))
		w.State = w.Reader.State
		_, err := w.Toggle()
		if !errors.Is(err, domain.ErrStopBeforeStart) {
			t.Errorf("Toggle: got %v, want ErrStopBeforeStart", err)
		}
	})
	t.Run("Edit", func(t *testing.T) {
		w := mkWriter(now)
		err := w.Edit(now, 0, future, now) // newStop before newStart
		if !errors.Is(err, domain.ErrStopBeforeStart) {
			t.Errorf("Edit: got %v, want ErrStopBeforeStart", err)
		}
	})
	t.Run("AddManual", func(t *testing.T) {
		w := mkWriter(now)
		err := w.AddManual(now, future, now) // stop before start
		if !errors.Is(err, domain.ErrStopBeforeStart) {
			t.Errorf("AddManual: got %v, want ErrStopBeforeStart", err)
		}
	})
}

func TestSessionWriter_StopAt_ExplicitTime(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(8 * time.Hour)
	stopT := start.Add(4 * time.Hour) // explicit shorter stop
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	s, err := w.StopAt(stopT)
	if err != nil {
		t.Fatal(err)
	}
	if s.Elapsed != 4*time.Hour {
		t.Errorf("StopAt: Elapsed = %v, want 4h", s.Elapsed)
	}
}

func TestSessionWriter_Pause_StopsAndMarks(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	s, err := w.Pause()
	if err != nil {
		t.Fatal(err)
	}
	if s.Elapsed != 2*time.Hour {
		t.Errorf("Pause Elapsed = %v", s.Elapsed)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Active != nil {
		t.Error("active should be cleared by pause")
	}
	if state.Pause == nil {
		t.Error("pause marker should be set")
	}
}

func TestSessionWriter_Pause_NoActiveIsNoOp(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	w := mkWriter(now)
	s, err := w.Pause()
	if err != nil {
		t.Fatal(err)
	}
	if s.Elapsed != 0 {
		t.Errorf("idle Pause should return zero session, got %v", s.Elapsed)
	}
}

// TestSessionWriter_Pause_StopBeforeStart_IsIdempotent pins the
// idempotent treatment of an NTP-backwards-jump (clock-now before the
// active session's start instant). Pause swallows ErrStopBeforeStart
// for the same reason it swallows ErrNoActiveSession: the tmux pause
// binding fires blindly and a red error flash for a transient clock
// glitch is wrong UX. The active state must NOT be cleared — the next
// Pause/Stop after the clock recovers should record the session.
func TestSessionWriter_Pause_StopBeforeStart_IsIdempotent(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	future := now.Add(time.Hour) // active started after clock-now
	w := mkWriter(now, withActive(future))
	w.State = w.Reader.State

	s, err := w.Pause()
	if err != nil {
		t.Fatalf("Pause must swallow ErrStopBeforeStart, got %v", err)
	}
	if !s.Start.IsZero() || s.Elapsed != 0 {
		t.Errorf("Pause on stop<start should return zero Session, got %+v", s)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Active == nil || !state.Active.Equal(future) {
		t.Errorf("active marker must be preserved through idempotent Pause; got %v, want %v", state.Active, future)
	}
	if state.Pause != nil {
		t.Errorf("pause marker must NOT be set when Pause was a no-op; got %v", state.Pause)
	}
}

func TestSessionWriter_Resume_ClearsPauseMarker(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	pause := now.Add(-time.Hour)
	w := mkWriter(now, withPause(pause))
	w.State = w.Reader.State

	if err := w.Resume(); err != nil {
		t.Fatal(err)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Pause != nil {
		t.Errorf("pause marker should be cleared, got %v", state.Pause)
	}
	if state.Active == nil || !state.Active.Equal(now) {
		t.Errorf("active should be set to now, got %v", state.Active)
	}
}

func TestSessionWriter_Resume_AlreadyRunningIsIdempotent(t *testing.T) {
	// CLAUDE.md: tmux bindings invoke worktime verbs blindly without
	// checking exit codes. Resume on a running session must not flash
	// red — the user already has the state they wanted. The pause
	// marker (if any) still gets cleared.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	pause := now.Add(-30 * time.Minute)
	active := now.Add(-time.Hour)
	w := mkWriter(now, func(r *usecase.WorktimeReader) {
		r.State = &testutil.FakeActiveSessionStore{Active: &active, Pause: &pause}
	})
	w.State = w.Reader.State
	if err := w.Resume(); err != nil {
		t.Fatalf("Resume should be idempotent on running session, got %v", err)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if state.Pause != nil {
		t.Errorf("pause marker should be cleared even when already running, got %v", state.Pause)
	}
	if state.Active == nil || !state.Active.Equal(active) {
		t.Errorf("active start should be untouched (%v), got %v", active, state.Active)
	}
}

func TestSessionWriter_Toggle_StartsWhenIdle(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	w := mkWriter(now)
	msg, err := w.Toggle()
	if err != nil {
		t.Fatal(err)
	}
	if msg != "gestartet" {
		t.Errorf("got %q, want 'gestartet'", msg)
	}
}

func TestSessionWriter_Toggle_StopsWhenRunning(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	msg, err := w.Toggle()
	if err != nil {
		t.Fatal(err)
	}
	if msg == "" {
		t.Errorf("Toggle running should return descriptive message, got %q", msg)
	}
}

func TestSessionWriter_CorrectStart(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	original := now.Add(-2 * time.Hour)
	corrected := now.Add(-3 * time.Hour)
	w := mkWriter(now, withActive(original))
	w.State = w.Reader.State

	if err := w.CorrectStart(corrected); err != nil {
		t.Fatal(err)
	}
	state := w.State.(*testutil.FakeActiveSessionStore)
	if !state.Active.Equal(corrected) {
		t.Errorf("active = %v, want %v", state.Active, corrected)
	}
}

func TestSessionWriter_CorrectStart_NoActiveFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	if err := w.CorrectStart(now); !errors.Is(err, domain.ErrNoActiveSession) {
		t.Errorf("expected ErrNoActiveSession, got %v", err)
	}
}
