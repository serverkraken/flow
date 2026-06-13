package usecase_test

// Error-propagation tests for SessionWriter: every store / state
// failure mode bubbles up through the appropriate verb. Driven by the
// flakySessionStore / flakyActiveStore helpers in
// session_writer_test.go.

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

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

// TestSessionWriter_Stop_MultiMidnightRetryDoesNotDuplicate guards
// review finding B1: a Stop spanning multiple midnights used to loop
// Append per part. If part N failed, parts 1..N-1 were on disk and the
// natural retry path duplicated them (SplitAtMidnight is deterministic).
// Plan-B follow-up #1 moved persistence to per-row Upsert with a
// dedupeSessionParts pre-pass against the on-disk slice, so a retry
// sees either zero or all rows from the previous attempt — never a
// duplicate.
func TestSessionWriter_Stop_MultiMidnightRetryDoesNotDuplicate(t *testing.T) {
	start := time.Date(2026, 4, 28, 22, 0, 0, 0, time.Local)
	now := time.Date(2026, 4, 30, 1, 0, 0, 0, time.Local) // crosses 2 midnights → 3 parts
	w := mkWriter(now, withActive(start))
	w.State = w.Reader.State

	// First attempt: the first Upsert fails. State stays "active".
	store := &flakySessionStore{FailOn: "Upsert"}
	w.Sessions = store
	if _, err := w.Stop(); err == nil {
		t.Fatal("expected error from Upsert")
	}
	if got := len(store.Sessions); got != 0 {
		t.Errorf("first attempt: %d sessions on disk after failure, want 0", got)
	}

	// Retry against a healthy store: same active marker, same input, all
	// 3 parts now persist exactly once.
	store.FailOn = ""
	if _, err := w.Stop(); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if got := len(store.Sessions); got != 3 {
		t.Errorf("retry: %d sessions, want 3 (one per midnight slice)", got)
	}
}

func TestSessionWriter_Delete_StoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	store := &flakySessionStore{
		Sessions: []domain.Session{{ID: "delerr-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)}},
		FailOn:   "Delete",
	}
	w.Sessions = store
	if err := w.Delete(d, 0); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_SetTag_StoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	store := &flakySessionStore{
		Sessions: []domain.Session{{ID: "settagerr-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)}},
		FailOn:   "Upsert",
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

// TestSessionWriter_Stop_ClearActiveFailsRetryDoesNotDuplicate pins
// the round4 idempotency fix: when a prior Stop succeeded at
// AppendBatch but failed at ClearActive, the session rows are on disk
// but the active marker stayed set. A retry must NOT re-write those
// rows (Sessions would carry duplicates of the same instant span).
// Implemented via dedupeSessionParts inside Stop's closure.
func TestSessionWriter_Stop_ClearActiveFailsRetryDoesNotDuplicate(t *testing.T) {
	start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	now := start.Add(2 * time.Hour) // single-part stop, simpler to assert
	w := mkWriter(now, withActive(start))
	state := &flakyActiveStore{Active: &start, FailOn: "ClearActive"}
	w.State = state
	store := &testutil.FakeSessionStore{}
	w.Sessions = store
	w.Reader.Sessions = store

	// First attempt: AppendBatch succeeds, ClearActive fails.
	if _, err := w.Stop(); err == nil {
		t.Fatal("expected error from ClearActive on first attempt")
	}
	if got := len(store.Sessions); got != 1 {
		t.Fatalf("first attempt should persist 1 session before ClearActive fails, got %d", got)
	}

	// Retry against a recovered ClearActive — must NOT duplicate.
	state.FailOn = ""
	if _, err := w.Stop(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := len(store.Sessions); got != 1 {
		t.Errorf("retry: %d sessions on disk, want 1 (dedupe must filter the duplicate)", got)
	}
	if state.Active != nil {
		t.Error("retry: active marker should be cleared")
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
