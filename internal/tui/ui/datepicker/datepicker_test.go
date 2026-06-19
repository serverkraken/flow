package datepicker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
)

func TestDatepicker_valueRoundTrip(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	if m.Value() != "2026-07-20" {
		t.Fatalf("Value = %q, want 2026-07-20", m.Value())
	}
	if err := m.SetValue("2024-02-29"); err != nil {
		t.Fatalf("SetValue err: %v", err)
	}
	if m.Value() != "2024-02-29" {
		t.Fatalf("after SetValue, Value = %q", m.Value())
	}
	if err := m.SetValue("not-a-date"); err == nil {
		t.Fatal("SetValue should reject bad input")
	}
}

func TestDatepicker_viewShowsSegmentsAndWeekday(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	v := m.View()
	if !strings.Contains(v, "2026") || !strings.Contains(v, "07") || !strings.Contains(v, "20") {
		t.Fatalf("view missing date segments: %q", v)
	}
	if !strings.Contains(v, "Mo") { // 2026-07-20 is a Monday
		t.Fatalf("view should show weekday Mo: %q", v)
	}
}
