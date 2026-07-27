package shell_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// nLineRoute renders exactly n lines, ignoring f.Height — mirroring routes like
// Home/Woche/Stats/Frei/Export (which don't read f.Height) and Today's
// fitHeight underfill when sessions are few.
type nLineRoute struct {
	stubRoute
	n int
}

func (r nLineRoute) Update(tea.Msg) (shell.Route, tea.Cmd) { return r, nil }
func (r nLineRoute) View(f shell.Frame) string {
	lines := make([]string, r.n)
	for i := range lines {
		lines[i] = fmt.Sprintf("body%d", i)
	}
	return strings.Join(lines, "\n")
}

// The shell must pin the keyhint footer to the bottom row of the terminal
// regardless of how many lines the active route renders: a route that
// underfills its height budget must not leave the footer floating mid-screen,
// and one that overfills must not push the footer off the bottom.
func TestShell_footerPinnedToBottom(t *testing.T) {
	const w, h = 80, 24
	for _, bodyLines := range []int{0, 5, 21, 40} {
		r := nLineRoute{
			stubRoute: stubRoute{title: "T", hints: []keyhint.Hint{{Key: "x", Desc: "y"}}},
			n:         bodyLines,
		}
		s := shell.New(nil, "u", theme.Default).WithTabs([]shell.Route{r})
		next, _ := s.Update(tea.WindowSizeMsg{Width: w, Height: h})
		v := next.(shell.Shell).View()
		lines := strings.Split(v.Content, "\n")

		if len(lines) != h {
			t.Errorf("body=%d lines: view height = %d, want %d (footer not pinned)", bodyLines, len(lines), h)
		}
		// The last row must be the footer keyhint (contains the hint key "x").
		if last := lines[len(lines)-1]; !strings.Contains(last, "x") {
			t.Errorf("body=%d lines: last row = %q, want the footer keyhint", bodyLines, last)
		}
	}
}
