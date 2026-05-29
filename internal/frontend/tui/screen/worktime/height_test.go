package worktime_test

// Height-invariant integration tests — the regression guard for the
// "Text verschwindet" bug: on a terminal shorter than a tab's content the
// altscreen used to scroll the headline + tab strip off the top. Every
// tab now routes its body through fitHeight, so at any terminal height the
// rendered worktime View must (a) never exceed that height (nothing scrolls
// off) and (b) keep the tab's identity anchor visible (context is never
// lost when the window shrinks).

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

// heightInvariantHeights spans terminal heights from absurdly short to
// comfortable. The short end forces the window/overflow-marker path; the
// tall end must still render every pinned region.
var heightInvariantHeights = []int{8, 10, 12, 16, 20, 24}

// assertHeightInvariant re-renders the already-loaded root at each test
// height and checks the two invariants. footerAnchor, when non-empty,
// must reappear once the terminal is tall enough (≥16) to pin the footer
// hint strip beneath the windowed body.
func assertHeightInvariant(t *testing.T, m tea.Model, anchor, footerAnchor string) {
	t.Helper()
	for _, h := range heightInvariantHeights {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: h})
		out := updated.View().Content
		if n := len(strings.Split(out, "\n")); n > h {
			t.Errorf("height %d: View rendered %d lines, exceeds budget (overflow):\n%s", h, n, out)
		}
		if !strings.Contains(strings.ToLower(out), strings.ToLower(anchor)) {
			t.Errorf("height %d: identity anchor %q missing — context lost:\n%s", h, anchor, out)
		}
		if footerAnchor != "" && h >= 16 && !strings.Contains(out, footerAnchor) {
			t.Errorf("height %d: footer anchor %q missing — hints dropped while room remained:\n%s", h, footerAnchor, out)
		}
	}
}

func TestHeightInvariant_Heute(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	// 25 short sessions overflow even a 24-row terminal so the session
	// list is always the windowed middle.
	for i := 0; i < 25; i++ {
		start := today.Add(time.Duration(i*20) * time.Minute)
		r.sessions.Sessions = append(r.sessions.Sessions, domain.Session{
			Date: today, Start: start, Stop: start.Add(15 * time.Minute),
			Elapsed: 15 * time.Minute,
		})
	}
	m := loadedHeute(t, r)
	assertHeightInvariant(t, m, "01.05.2026", "bewegen")
}

func TestHeightInvariant_Woche(t *testing.T) {
	r := newRig(t)
	// Woche is structurally bounded (≤7 day rows + KPI block), but the
	// short terminals still force the window path. Seed each weekday so the
	// day rows carry real totals.
	mon := isoMondayOf(r.clock.T)
	for i := 0; i < 5; i++ {
		day := mon.AddDate(0, 0, i)
		start := day.Add(9 * time.Hour)
		r.sessions.Sessions = append(r.sessions.Sessions, domain.Session{
			Date: day, Start: start, Stop: start.Add(8 * time.Hour),
			Elapsed: 8 * time.Hour,
		})
	}
	m := loadedWoche(t, r)
	assertHeightInvariant(t, m, "KW 18", "bewegen")
}

func TestHeightInvariant_History(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	// 40 day-records across several ISO weeks overflow the list at every
	// short height (and well past height 24 with the KW section headers).
	for i := 0; i < 40; i++ {
		day := today.AddDate(0, 0, -i)
		start := day.Add(9 * time.Hour)
		r.sessions.Sessions = append(r.sessions.Sessions, domain.Session{
			Date: day, Start: start, Stop: start.Add(7 * time.Hour),
			Elapsed: 7 * time.Hour,
		})
	}
	m := loadedHistory(t, r)
	assertHeightInvariant(t, m, "Tage", "bewegen")
}

func TestHeightInvariant_Frei(t *testing.T) {
	r := newRig(t)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	// 30 weekly vacation entries fill the current year's list past any
	// test height.
	for i := 0; i < 30; i++ {
		day := jan.AddDate(0, 0, i*7)
		if err := r.dayoffs.Add(domain.DayOff{
			Date: day, Kind: domain.KindVacation, Label: "Urlaub",
		}); err != nil {
			t.Fatalf("seed dayoff %d: %v", i, err)
		}
	}
	m := loadedFrei(t, r)
	assertHeightInvariant(t, m, "Frei 2026", "bewegen")
}
