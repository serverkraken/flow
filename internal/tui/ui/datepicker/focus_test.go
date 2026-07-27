package datepicker_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
)

// TestDatepicker_FocusBlur exercises the Focus, Blur, and Focused methods
// which are at 0% coverage.
func TestDatepicker_FocusBlur(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)

	// Initial state: not focused.
	if m.Focused() {
		t.Error("new datepicker should not be focused initially")
	}

	// Focus → Focused() returns true.
	m.Focus()
	if !m.Focused() {
		t.Error("after Focus(), Focused() should return true")
	}

	// Blur → Focused() returns false.
	m.Blur()
	if m.Focused() {
		t.Error("after Blur(), Focused() should return false")
	}
}
