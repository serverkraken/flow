package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal Route for back tests; capabilities toggled per case.
type stubRoute struct {
	text   bool
	backOK bool
}

func (s stubRoute) Title() string                   { return "stub" }
func (s stubRoute) Init() tea.Cmd                   { return nil }
func (s stubRoute) Update(tea.Msg) (Route, tea.Cmd) { return s, nil }
func (s stubRoute) View(Frame) string               { return "" }
func (s stubRoute) KeyHints() []keyhint.Hint        { return nil }
func (s stubRoute) CapturesText() bool              { return s.text }
func (s stubRoute) Back() (Route, tea.Cmd, bool)    { return s, nil, s.backOK }

func TestResolveBack(t *testing.T) {
	cases := []struct {
		name    string
		top     Route
		depth   int
		overlay bool
		want    BackAction
	}{
		{"overlay closes first", stubRoute{}, 2, true, BackOverlay},
		{"text field forwards", stubRoute{text: true}, 2, false, BackForward},
		{"route internal back", stubRoute{backOK: true}, 2, false, BackRoute},
		{"pop when deep", stubRoute{}, 2, false, BackPop},
		{"quit at root", stubRoute{}, 1, false, BackQuit},
	}
	for _, tc := range cases {
		if got := ResolveBack(tc.top, tc.depth, tc.overlay); got != tc.want {
			t.Errorf("%s: ResolveBack = %v, want %v", tc.name, got, tc.want)
		}
	}
}
