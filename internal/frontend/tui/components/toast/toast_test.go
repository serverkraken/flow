package toast_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

var testPalette = theme.Load()

func TestNew_VisibleInitially(t *testing.T) {
	t.Parallel()
	m := toast.New("saved", 3*time.Second, testPalette)
	if !m.Visible() {
		t.Error("toast should be visible after creation")
	}
}

func TestView_ContainsText(t *testing.T) {
	t.Parallel()
	m := toast.New("Branch erstellt", 3*time.Second, testPalette)
	if !strings.Contains(m.View(), "Branch erstellt") {
		t.Error("view missing toast text")
	}
}

func TestUpdate_DismissedMsg_HidesToast(t *testing.T) {
	t.Parallel()
	m := toast.New("done", 1*time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if m.Visible() {
		t.Error("toast should be hidden after DismissedMsg")
	}
}

func TestView_EmptyAfterDismiss(t *testing.T) {
	t.Parallel()
	m := toast.New("done", 1*time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if m.View() != "" {
		t.Errorf("expected empty view after dismiss, got %q", m.View())
	}
}

func TestInit_ReturnsCmd(t *testing.T) {
	t.Parallel()
	m := toast.New("hi", 2*time.Second, testPalette)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a tick command")
	}
}

// TestKindGlyphs verifies A11y-2 from the design-system audit: every
// toast kind carries a distinct glyph in addition to its colour, so a
// NO_COLOR or colour-blind viewer still gets the semantic flavour.
func TestKindGlyphs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		make   func() toast.Model
		glyph  string
		others []string // glyphs that must NOT appear
	}{
		{"success", func() toast.Model { return toast.NewSuccess("ok", testPalette) }, "✓", []string{"▲", "✗", "›"}},
		{"warning", func() toast.Model { return toast.NewWarning("careful", testPalette) }, "▲", []string{"✓", "✗", "›"}},
		{"danger", func() toast.Model { return toast.NewDanger("failed", testPalette) }, "✗", []string{"✓", "▲", "›"}},
		{"info", func() toast.Model { return toast.NewInfo("fyi", testPalette) }, "›", []string{"✓", "▲", "✗"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.make().View()
			if !strings.Contains(out, tc.glyph) {
				t.Errorf("%s: missing glyph %q in %q", tc.name, tc.glyph, out)
			}
			for _, g := range tc.others {
				if strings.Contains(out, g) {
					t.Errorf("%s: stray glyph %q from another kind in %q", tc.name, g, out)
				}
			}
		})
	}
}
