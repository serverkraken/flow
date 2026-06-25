package worktime

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// makeRunningRoute builds a TodayRoute with a running session started at
// 09:00 UTC on 2026-06-14 (fixedNow is 12:00 UTC that day) and the
// adjust-start dialog already open.
func makeRunningRoute(t *testing.T, f *fakeAPI) *TodayRoute {
	t.Helper()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	r := NewTodayRoute(f, fixedNow, theme.Default, nil)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"
	res, _ := r.openAdjustStart()
	dr := res.(*TodayRoute)
	if dr.dialog != dialogEditStart {
		t.Fatal("could not open adjust-start dialog")
	}
	return dr
}

func TestOpenAdjustStart_prefillsCurrentStart(t *testing.T) {
	r := makeRunningRoute(t, &fakeAPI{})
	if got := r.adjust.input.Value(); got != "09:00" {
		t.Fatalf("prefill = %q want 09:00", got)
	}
}

func TestSubmitAdjustStart_validCallsEditWithNilStop(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("08:30")

	cmd := r.submitAdjustStart()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(reloadMsg); !ok {
		t.Fatal("valid submit should yield reloadMsg")
	}
	if f.edited != "run1" {
		t.Fatalf("EditSession not called for run1: %q", f.edited)
	}
	if f.editStop != nil {
		t.Fatal("stop must be nil so the session keeps running")
	}
	want := time.Date(2026, 6, 14, 8, 30, 0, 0, time.UTC)
	if !f.editStart.Equal(want) {
		t.Fatalf("start = %v want %v", f.editStart, want)
	}
	if r.dialog != dialogNone {
		t.Fatal("dialog should close after a valid submit")
	}
}

func TestSubmitAdjustStart_invalidKeepsDialogNoCall(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("99:99")

	r.submitAdjustStart()
	if f.edited != "" {
		t.Fatal("invalid HH:MM must not call EditSession")
	}
	if r.dialog != dialogEditStart {
		t.Fatal("dialog should stay open on invalid input")
	}
}

func TestSubmitAdjustStart_futureNoCall(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("13:00") // fixedNow is 12:00 UTC

	r.submitAdjustStart()
	if f.edited != "" {
		t.Fatal("future start must not call EditSession")
	}
}

func TestHandleAdjustStartKey_EscCancels(t *testing.T) {
	r := makeRunningRoute(t, &fakeAPI{})
	res, _ := r.handleAdjustStartKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if res.(*TodayRoute).dialog != dialogNone {
		t.Fatal("esc should close the dialog")
	}
}
