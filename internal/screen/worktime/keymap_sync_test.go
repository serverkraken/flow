package worktime

import (
	"strings"
	"testing"

	tk "github.com/serverkraken/tui-kit/theme"
)

// TestHelpMentionsImportantKeys is a drift guard: each key the user is
// expected to discover must show up in the in-screen `?` help. Brittle by
// design — when a key is added, the test forces the author to also document
// it. Replaces a fully-structured KeyMap until the bigger refactor lands.
func TestHelpMentionsImportantKeys(t *testing.T) {
	m := New(tk.Palette{})
	m.width = 120
	body := strings.Join(m.renderHelpBody(116), "\n")

	// Each entry: token to look for, plus a label that goes into the failure
	// message so a future debugger sees "Y ('gestern drilldown')" instead of
	// just a raw character.
	wanted := map[string]string{
		"tab":   "tab navigation",
		"1":     "tab-1 jump",
		"2":     "tab-2 jump",
		"3":     "tab-3 jump",
		"4":     "tab-4 jump",
		"s":     "start/stop/resume",
		"S":     "force-start",
		"p":     "pause",
		"C":     "correct start time",
		"e":     "manual entry",
		"E":     "edit session",
		"d":     "delete session",
		"u":     "undo last delete",
		"t":     "set tag",
		"N":     "set note",
		"n":     "attach kompendium note",
		"o":     "view note",
		"O":     "open note in editor",
		"D":     "detach note",
		"f":     "focus mode",
		"Y":     "yesterday drilldown",
		"a":     "add dayoff",
		"A":     "quick today=vacation",
		"K":     "quick today=sick",
		"B":     "Bundesland sync",
		"v":     "history list/heatmap toggle",
		"/":     "history filter",
		"y":     "yank day",
		"T":     "now-jump (week/dayoffs/history)",
		"[":     "step back filter",
		"]":     "step forward filter",
		"g":     "cursor top",
		"G":     "cursor bottom",
		"r":     "reload",
		"?":     "help itself",
		"b":     "back to previous tab",
		"+1h30m": "stop-field duration shorthand",
		"Ctrl+T": "session template cycle",
	}

	for tok, what := range wanted {
		if !containsKey(body, tok) {
			t.Errorf("renderHelpBody does not mention %q (%s) — add a line to renderHelpBody when introducing the key", tok, what)
		}
	}
}

// TestFootersMentionGlobalActions verifies that view footers reference the
// keys they handle. Keeps the footer hint accurate even if a handler grows
// new keys.
func TestFootersMentionGlobalActions(t *testing.T) {
	m := New(tk.Palette{})
	m.width = 120

	cases := []struct {
		name      string
		footerKey string
		footer    string
	}{
		{"today", "s", m.todayFooter()},
		{"today", "e", m.todayFooter()},
		{"today", "tab", m.todayFooter()},
		{"today", "?", m.todayFooter()},
	}
	for _, c := range cases {
		if !containsKey(c.footer, c.footerKey) {
			t.Errorf("%s footer missing key %q — footer was: %q", c.name, c.footerKey, c.footer)
		}
	}
}

// containsKey returns true when `body` references the key token. Two-pass
// tokenisation: first whitespace, then keyboard-cheatsheet inner separators
// (/, comma, middle-dot, |, ⇧). The two passes preserve the difference
// between a standalone "/" key and "/" as part of "tab/1/2/3/4".
func containsKey(body, tok string) bool {
	innerSeps := func(r rune) bool {
		switch r {
		case '/', ',', '·', '⇧', '|':
			return true
		}
		return false
	}
	for _, fld := range strings.Fields(body) {
		if fld == tok {
			return true
		}
		for _, sub := range strings.FieldsFunc(fld, innerSeps) {
			if sub == tok {
				return true
			}
		}
	}
	return false
}
