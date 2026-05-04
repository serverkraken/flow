package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkWriter(now time.Time, opts ...readerOpt) *usecase.SessionWriter {
	reader := mkReader(now, nil, opts...)
	return &usecase.SessionWriter{
		Sessions: reader.Sessions,
		State:    reader.State,
		Lock:     &testutil.FakeLock{},
		Reader:   reader,
		Clock:    reader.Clock,
	}
}

// — lifecycle —

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
		t.Error("expected error for stop before start")
	}
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

// — manual edits —

func TestSessionWriter_AddManual_HappyPath(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.AddManual(d, d.Add(9*time.Hour), d.Add(11*time.Hour)); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 || store.Sessions[0].Elapsed != 2*time.Hour {
		t.Errorf("got %+v", store.Sessions)
	}
}

func TestSessionWriter_AddManual_StopBeforeStartFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	err := w.AddManual(d, d.Add(11*time.Hour), d.Add(9*time.Hour))
	if err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_AddManual_OverlapDetected(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	// Pre-seed an existing 09:00–11:00 session.
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}
	w.Reader.Sessions = w.Sessions

	err := w.AddManual(d, d.Add(10*time.Hour), d.Add(12*time.Hour))
	if !errors.Is(err, domain.ErrOverlap) {
		t.Errorf("expected ErrOverlap, got %v", err)
	}
}

func TestSessionWriter_Edit_PreservesTagAndNote(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour), Tag: "deep", Note: "auth"},
	}}
	w.Reader.Sessions = w.Sessions

	if err := w.Edit(d, 0, d.Add(9*time.Hour), d.Add(12*time.Hour)); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(store.Sessions))
	}
	got := store.Sessions[0]
	if got.Stop.Hour() != 12 {
		t.Errorf("stop not updated: got %v", got.Stop)
	}
	if got.Tag != "deep" || got.Note != "auth" {
		t.Errorf("tag/note clobbered: %+v", got)
	}
}

func TestSessionWriter_Edit_OverlapDetected(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
		{Date: d, Start: d.Add(13 * time.Hour), Stop: d.Add(15 * time.Hour)},
	}}
	w.Reader.Sessions = w.Sessions

	// Edit session 0 to span 10:00–14:00 — overlaps session 1.
	err := w.Edit(d, 0, d.Add(10*time.Hour), d.Add(14*time.Hour))
	if !errors.Is(err, domain.ErrOverlap) {
		t.Errorf("expected ErrOverlap, got %v", err)
	}
}

func TestSessionWriter_Edit_StopBeforeStartFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.Edit(d, 0, d.Add(11*time.Hour), d.Add(9*time.Hour)); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Delete(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
		{Date: d, Start: d.Add(13 * time.Hour), Stop: d.Add(15 * time.Hour)},
	}}
	if err := w.Delete(d, 0); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 {
		t.Fatalf("expected 1 session left, got %d", len(store.Sessions))
	}
	if store.Sessions[0].Start.Hour() != 13 {
		t.Errorf("wrong session deleted: %+v", store.Sessions[0])
	}
}

func TestSessionWriter_SetTag(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}

	if err := w.SetTag(d, 0, "deep\ttab\nnewline"); err != nil {
		t.Fatal(err)
	}
	got := w.Sessions.(*testutil.FakeSessionStore).Sessions[0].Tag
	// tabs/newlines in tag must be sanitized to spaces.
	if got != "deep tab newline" {
		t.Errorf("Tag = %q, want sanitized", got)
	}
}

func TestSessionWriter_SetNote(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}

	if err := w.SetNote(d, 0, "  some note  "); err != nil {
		t.Fatal(err)
	}
	if got := w.Sessions.(*testutil.FakeSessionStore).Sessions[0].Note; got != "some note" {
		t.Errorf("Note = %q, want trimmed", got)
	}
}

// — error propagation —

func TestSessionWriter_Start_StoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.State = &testutil.FakeActiveSessionStore{Err: errors.New("boom")}
	if err := w.Start(now); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_AddManual_StoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	w.Reader.Sessions = w.Sessions
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.AddManual(d, d.Add(9*time.Hour), d.Add(10*time.Hour)); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Stop_StateLoadErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.State = &testutil.FakeActiveSessionStore{Err: errors.New("boom")}
	if _, err := w.Stop(); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Toggle_StateLoadErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.State = &testutil.FakeActiveSessionStore{Err: errors.New("boom")}
	if _, err := w.Toggle(); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Edit_OverlapLookupErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	w.Reader.Sessions = w.Sessions
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.Edit(d, 0, d.Add(9*time.Hour), d.Add(11*time.Hour)); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Delete_LoadErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	d := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	if err := w.Delete(d, 0); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_StartForce_StoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	w.State = &testutil.FakeActiveSessionStore{Err: errors.New("boom")}
	if err := w.StartForce(now); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_AddManual_AppendErr(t *testing.T) {
	// Sessions has Err set, but only Append fails — LoadAll must succeed
	// for the overlap check to pass. Use a custom fake.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	store := &flakySessionStore{
		Sessions: nil,
		FailOn:   "Append",
	}
	w.Sessions = store
	w.Reader.Sessions = store
	if err := w.AddManual(d, d.Add(9*time.Hour), d.Add(10*time.Hour)); err == nil {
		t.Error("expected error from Append")
	}
}

func TestSessionWriter_Stop_AppendErr(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State
	store := &flakySessionStore{FailOn: "Append"}
	w.Sessions = store
	if _, err := w.Stop(); err == nil {
		t.Error("expected error from Append")
	}
}

func TestSessionWriter_Toggle_StopBubbleErr(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State
	store := &flakySessionStore{FailOn: "Append"}
	w.Sessions = store
	if _, err := w.Toggle(); err == nil {
		t.Error("expected error from Stop->Append")
	}
}

func TestSessionWriter_Pause_StopErrPropagates(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State
	store := &flakySessionStore{FailOn: "Append"}
	w.Sessions = store
	if _, err := w.Pause(); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Delete_RewriteErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	store := &flakySessionStore{
		Sessions: []domain.Session{{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)}},
		FailOn:   "Rewrite",
	}
	w.Sessions = store
	if err := w.Delete(d, 0); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_SetTag_RewriteErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	store := &flakySessionStore{
		Sessions: []domain.Session{{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)}},
		FailOn:   "Rewrite",
	}
	w.Sessions = store
	if err := w.SetTag(d, 0, "foo"); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Toggle_StartErrPropagates(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	// Idle (no active) but State.SetActive fails.
	w.State = &flakyActiveStore{FailOn: "SetActive"}
	if _, err := w.Toggle(); err == nil {
		t.Error("expected error from Start path of Toggle")
	}
}

func TestSessionWriter_Stop_ClearActiveErr(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour)
	w := mkWriter(now, withActive(start))
	store := &flakyActiveStore{Active: &start, FailOn: "ClearActive"}
	w.State = store
	if _, err := w.Stop(); err == nil {
		t.Error("expected error from ClearActive")
	}
}

func TestSessionWriter_Delete_KeepsOtherDates(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d1, _ := time.ParseInLocation("2006-01-02", "2026-04-27", time.Local)
	d2, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{Date: d1, Start: d1.Add(9 * time.Hour), Stop: d1.Add(11 * time.Hour)},
		{Date: d2, Start: d2.Add(9 * time.Hour), Stop: d2.Add(11 * time.Hour)},
	}}
	if err := w.Delete(d2, 0); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 || !store.Sessions[0].Date.Equal(d1) {
		t.Errorf("only d2's session should be deleted, got %+v", store.Sessions)
	}
}

func TestSessionWriter_SetTag_LoadErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	w.Sessions = &flakySessionStore{FailOn: "LoadAll"}
	if err := w.SetTag(d, 0, "x"); err == nil {
		t.Error("expected error from LoadAll")
	}
}

// flakySessionStore fails only on the named method; other methods succeed
// against the in-memory Sessions slice.
type flakySessionStore struct {
	Sessions []domain.Session
	FailOn   string
}

func (f *flakySessionStore) LoadAll() ([]domain.Session, error) {
	if f.FailOn == "LoadAll" {
		return nil, errors.New("boom")
	}
	out := make([]domain.Session, len(f.Sessions))
	copy(out, f.Sessions)
	return out, nil
}

func (f *flakySessionStore) LoadFiltered(keep func(domain.Session) bool) ([]domain.Session, error) {
	if f.FailOn == "LoadFiltered" {
		return nil, errors.New("boom")
	}
	out := []domain.Session{}
	for _, s := range f.Sessions {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *flakySessionStore) Append(s domain.Session) error {
	if f.FailOn == "Append" {
		return errors.New("boom")
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

func (f *flakySessionStore) Rewrite(sessions []domain.Session) error {
	if f.FailOn == "Rewrite" {
		return errors.New("boom")
	}
	f.Sessions = make([]domain.Session, len(sessions))
	copy(f.Sessions, sessions)
	return nil
}

// flakyActiveStore fails only on the named method.
type flakyActiveStore struct {
	Active *time.Time
	Pause  *time.Time
	FailOn string
}

func (f *flakyActiveStore) GetActive() (*time.Time, error) {
	if f.FailOn == "GetActive" {
		return nil, errors.New("boom")
	}
	return f.Active, nil
}

func (f *flakyActiveStore) SetActive(t time.Time) error {
	if f.FailOn == "SetActive" {
		return errors.New("boom")
	}
	v := t
	f.Active = &v
	return nil
}

func (f *flakyActiveStore) ClearActive() error {
	if f.FailOn == "ClearActive" {
		return errors.New("boom")
	}
	f.Active = nil
	return nil
}

func (f *flakyActiveStore) GetPause() (*time.Time, error) {
	if f.FailOn == "GetPause" {
		return nil, errors.New("boom")
	}
	return f.Pause, nil
}

func (f *flakyActiveStore) SetPause(t time.Time) error {
	if f.FailOn == "SetPause" {
		return errors.New("boom")
	}
	v := t
	f.Pause = &v
	return nil
}

func (f *flakyActiveStore) ClearPause() error {
	if f.FailOn == "ClearPause" {
		return errors.New("boom")
	}
	f.Pause = nil
	return nil
}
