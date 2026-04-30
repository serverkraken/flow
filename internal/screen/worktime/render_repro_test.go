package worktime

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tk "github.com/serverkraken/tui-kit/theme"
)

// TestRenderAtNarrowWidth_NoOverflow renders every view (and a running-state
// variant of Today) at common tmux pane widths, then asserts that no line in
// the rendered output exceeds the terminal width. An over-wide line gets
// hard-wrapped by the terminal, which looks like a "doubled" overview / box
// to the user — that's the regression this guards against.
func TestRenderAtNarrowWidth_NoOverflow(t *testing.T) {
	cases := []struct {
		name  string
		setup func(Model) Model
	}{
		{"today_idle", func(m Model) Model { m.view = viewToday; return m }},
		{"today_running", func(m Model) Model {
			m.view = viewToday
			now := m.now.Add(-2 * time.Hour)
			m.day.Active = &now
			return m
		}},
		{"week", func(m Model) Model { m.view = viewWeek; m.weekLoaded = true; return m }},
		{"history", func(m Model) Model { m.view = viewHistory; m.historyLoaded = true; return m }},
		{"dayoffs", func(m Model) Model { m.view = viewDayOffs; m.dayoffsLoaded = true; return m }},
	}
	widths := []int{40, 50, 60, 80, 100, 120}
	for _, c := range cases {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s_width%d", c.name, w), func(t *testing.T) {
				m := New(tk.Palette{})
				updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
				m = updated.(Model)
				m.loading = false
				m = c.setup(m)

				out := m.View()
				lines := strings.Split(out, "\n")
				over := 0
				for i, ln := range lines {
					if visibleWidth(ln) > w {
						over++
						if over <= 3 {
							t.Logf("line %d width=%d (>%d): %q", i, visibleWidth(ln), w, ln)
						}
					}
				}
				if over > 0 {
					t.Errorf("%s @ width %d: %d/%d lines exceed terminal width", c.name, w, over, len(lines))
				}
			})
		}
	}
}

// visibleWidth uses lipgloss.Width which understands ANSI sequences and East
// Asian Width — same metric the renderer uses, so this matches what the
// terminal actually displays.
func visibleWidth(s string) int {
	return lipgloss.Width(s)
}

// TestStDimMultilinePadsShorterLines pins the lipgloss behaviour we worked
// around: when a string passed through lipgloss.Render contains a newline,
// the shorter line gets padded with spaces to match the longer one. Caused
// the "doubled overview" bug — the padding leaked into the preceding tab
// bar via plain string concatenation. Production callers must therefore
// keep "\n" *outside* the stDim call. If lipgloss ever changes this, this
// test fails and the workaround can be simplified.
func TestStDimMultilinePadsShorterLines(t *testing.T) {
	out := stDim(tk.Palette{}, "\n  short.")
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	if visibleWidth(lines[0]) == 0 {
		t.Skip("lipgloss no longer pads multi-line styled strings — the multiline-stDim workaround is unnecessary; consider simplifying the renderHistory empty-state branches.")
	}
}
