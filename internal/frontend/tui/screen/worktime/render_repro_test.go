package worktime_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TestRenderAtNarrowWidth_NoOverflow renders every tab (and a running-state
// variant of Heute) at common tmux pane widths, then asserts that no line
// in the rendered output exceeds the terminal width. An over-wide line gets
// hard-wrapped by the terminal, which looks like a "doubled" overview /
// box to the user — that's the regression this guards against.
func TestRenderAtNarrowWidth_NoOverflow(t *testing.T) {
	// Each case sets up the rig (e.g. a running session for heute_running) and
	// names the tab key the rig should land on after Init drains.
	cases := []struct {
		name  string
		setup func(rig)
		tab   string
	}{
		{"heute_idle", func(_ rig) {}, "1"},
		{"heute_running", func(r rig) {
			start := r.clock.T.Add(-2 * time.Hour)
			r.active.Active = &start
		}, "1"},
		{"woche", func(_ rig) {}, "2"},
		{"history", func(_ rig) {}, "3"},
		{"frei", func(_ rig) {}, "4"},
	}
	widths := []int{40, 50, 60, 80, 100, 120}

	for _, c := range cases {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s_width%d", c.name, w), func(t *testing.T) {
				r := newRig(t)
				c.setup(r)
				updated, _ := r.model.Update(tea.WindowSizeMsg{Width: w, Height: 30})
				loaded := drainCmd(t, updated, updated.Init())
				loaded, _ = loaded.Update(tea.KeyPressMsg{Text: c.tab})

				out := loaded.View()
				lines := strings.Split(out, "\n")
				over := 0
				for i, ln := range lines {
					if lipgloss.Width(ln) > w {
						over++
						if over <= 3 {
							t.Logf("line %d width=%d (>%d): %q", i, lipgloss.Width(ln), w, ln)
						}
					}
				}
				if over > 0 {
					t.Errorf("%s @ width %d: %d/%d lines exceed terminal width",
						c.name, w, over, len(lines))
				}
			})
		}
	}
}
