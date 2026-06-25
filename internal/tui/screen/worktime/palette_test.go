package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestPaletteEntries_onlyWhenRunning(t *testing.T) {
	r := NewTodayRoute(&fakeAPI{}, fixedNow, theme.Default, nil)
	if r.PaletteEntries() != nil {
		t.Fatal("no palette entry while idle")
	}
	start := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"

	e := r.PaletteEntries()
	if len(e) != 1 || e[0].Label != "Startzeit anpassen" {
		t.Fatalf("want 1 entry 'Startzeit anpassen', got %v", e)
	}
	if _, ok := e[0].Action().(adjustStartMsg); !ok {
		t.Fatal("entry action should yield adjustStartMsg")
	}
}

func TestUpdate_adjustStartMsgOpensDialog(t *testing.T) {
	start := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r := NewTodayRoute(&fakeAPI{}, fixedNow, theme.Default, nil)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"

	res, _ := r.Update(adjustStartMsg{})
	dr := res.(*TodayRoute)
	if dr.dialog != dialogEditStart {
		t.Fatalf("dialog = %v want dialogEditStart", dr.dialog)
	}
	if got := dr.adjust.input.Value(); got != "10:00" {
		t.Fatalf("prefill = %q want 10:00", got)
	}
}
