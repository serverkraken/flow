package toast_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

// Covers the SlotLine + SlotRows helpers and the Dismiss / NewDefault
// shortcuts. Each is small and orthogonal to the kind/glyph tests in
// toast_test.go; isolating them keeps that file focused on the
// rendering paths.

func TestNewDefault_UsesDefaultDuration(t *testing.T) {
	m := toast.NewDefault("ok", testPalette)
	if !m.Visible() {
		t.Errorf("NewDefault toast should be visible")
	}
	// Without exporting the duration we can't read it directly, but
	// Init returns a tick — a non-nil cmd proves the duration plumbed.
	if cmd := m.Init(); cmd == nil {
		t.Errorf("Init should return a tick cmd")
	}
	_ = toast.DefaultDuration // compile-time import sanity
}

func TestDismiss_ReturnsMatchingMessage(t *testing.T) {
	m := toast.New("hi", time.Second, testPalette)
	msg := m.Dismiss()
	// Sending the matching dismiss should hide the toast.
	m2, _ := m.Update(msg)
	if m2.Visible() {
		t.Errorf("matching Dismiss should hide toast")
	}
}

func TestSlotLine_NilOrInvisible_ReturnsEmpty(t *testing.T) {
	if got := toast.SlotLine(nil, "  "); got != "" {
		t.Errorf("SlotLine(nil) = %q, want empty", got)
	}
	m := toast.New("x", time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if got := toast.SlotLine(&m, "  "); got != "" {
		t.Errorf("SlotLine on dismissed toast should be empty, got %q", got)
	}
}

func TestSlotLine_VisibleIncludesIndent(t *testing.T) {
	m := toast.New("hi", time.Second, testPalette)
	got := toast.SlotLine(&m, ">>>")
	if !strings.HasPrefix(got, ">>>") {
		t.Errorf("SlotLine should prefix with indent, got %q", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("SlotLine should contain toast text, got %q", got)
	}
}

func TestSlotRows_NilOrInvisible_ReturnsNil(t *testing.T) {
	if got := toast.SlotRows(nil, "  "); got != nil {
		t.Errorf("SlotRows(nil) = %v, want nil", got)
	}
	m := toast.New("x", time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if got := toast.SlotRows(&m, "  "); got != nil {
		t.Errorf("SlotRows on dismissed should be nil, got %v", got)
	}
}

func TestSlotRows_VisibleReturnsTwoRows(t *testing.T) {
	m := toast.New("hello", time.Second, testPalette)
	rows := toast.SlotRows(&m, "  ")
	if len(rows) != 2 {
		t.Fatalf("SlotRows visible should return 2 rows, got %d", len(rows))
	}
	if rows[0] != "" {
		t.Errorf("row 0 should be empty separator, got %q", rows[0])
	}
	if !strings.Contains(rows[1], "hello") {
		t.Errorf("row 1 should contain text, got %q", rows[1])
	}
}
