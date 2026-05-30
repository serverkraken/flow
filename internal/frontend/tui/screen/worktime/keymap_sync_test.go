package worktime_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

// TestFooterDriftGuard asserts each sub-model's footer hint strip
// references the action keys it actually handles. Brittle by design — when
// a key is added to a sub-model's keymap, the test forces the author to
// also document it in footerHints(). Replaces the legacy
// renderHelpBody-driven drift guard now that the help screen lives only
// as a wave B+ deferred enhancement.
//
// Wave B+ keys (S, C, e, u, n, o, O, D, f, Y, y, r, ?, Ctrl+T) are
// deferred — when they land, add a case here. Tab-router keys (1/2/3/4,
// tab, b) live on the root model, not on a sub-model footer.
func TestFooterDriftGuard(t *testing.T) {
	cases := []struct {
		name  string
		setup func(rig)
		tab   string
		keys  []string
	}{
		{
			name:  "heute_idle",
			setup: func(_ rig) {},
			tab:   "1",
			// Off-session: `s, j/k, :, ?` — Phase-10 follow-up added
			// `? → Hilfe` (Skill §Keybind: ?-help must be discoverable
			// from every footer).
			keys: []string{"s", "j/k", ":", "?"},
		},
		{
			name: "heute_running",
			setup: func(r rig) {
				start := r.clock.T.Add(-30 * time.Minute)
				r.active.Active = &start
			},
			tab: "1",
			// Skill §Hint format ≤4: Top-Frequenz-Hints; `p` (pause)
			// wandert in den `?`-Overlay. Running ohne fokussierte
			// Session = off-session-Branch.
			keys: []string{"s", "j/k", ":", "?"},
		},
		{
			name: "heute_on_session",
			setup: func(r rig) {
				today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
				r.sessions.Sessions = []domain.Session{{
					Date: today, Start: today.Add(9 * time.Hour),
					Stop: today.Add(10 * time.Hour), Elapsed: time.Hour,
				}}
			},
			tab: "1",
			// Top-4 (s, j/k, enter, ?). Phase-10 follow-up: `:` wandert
			// in den `?`-Overlay, damit `? → Hilfe` selbst im Footer
			// stehen kann (Skill §Keybind: ?-help fixed slot).
			keys: []string{"s", "j/k", "enter", "?"},
		},
		{
			name:  "woche",
			setup: func(_ rig) {},
			tab:   "2",
			// Tab-Navigation 1/2/3/4 ist parent-level — gehört nicht in den
			// screen-level Footer (Skill §Hint format „context-relevant").
			keys: []string{"j/k", "g/G", ":", "?"},
		},
		{
			name:  "history",
			setup: func(_ rig) {},
			tab:   "3",
			// Phase-10 follow-up: `/` (filter) wandert in den `?`-Overlay
			// (universal-fixed-slot key, via `?` discoverable), damit
			// `? → Hilfe` selbst im Footer stehen kann. `:`-Aktions-Menü
			// und `T`/`F` bleiben ebenfalls im `?`-Overlay.
			keys: []string{"j/k", "enter", "v", "?"},
		},
		{
			name:  "frei",
			setup: func(_ rig) {},
			tab:   "4",
			// Phase-10 follow-up: `:`-Aktions-Menü wandert in den
			// `?`-Overlay (über Palette + `?` weiterhin erreichbar),
			// damit `? → Hilfe` selbst im Footer stehen kann.
			// h/l/[/], A/K/B/T sind im `?`-Overlay.
			keys: []string{"j/k", "a", "D", "?"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			c.setup(r)
			updated, _ := r.model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			loaded := drainCmd(t, updated, updated.Init())
			loaded, _ = loaded.Update(tea.KeyPressMsg{Text: c.tab})

			out := loaded.View().Content
			footer := lastFooterLines(out)
			for _, k := range c.keys {
				if !containsKey(footer, k) {
					t.Errorf("%s footer missing key %q — footer was:\n%s", c.name, k, footer)
				}
			}
		})
	}
}

// lastFooterLines returns the trailing dim hint lines of a rendered View
// (everything after the last blank line). The footer can wrap to multiple
// lines on narrow widths so a single-line read won't catch all keys.
func lastFooterLines(view string) string {
	lines := strings.Split(view, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return view
}

// containsKey returns true when `body` references the key token. Two-pass
// tokenisation: first whitespace, then keyboard-cheatsheet inner separators
// (/, comma, middle-dot, |, ⇧). The two passes preserve the difference
// between a standalone "/" key (matches via outer whitespace pass) and "/"
// as part of "j/k" (filtered out by the inner pass).
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
