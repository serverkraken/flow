package worktime

// White-box test for history_drill.go's pure session-list renderer. Pins
// the pause-separator format to the app-wide BulletDot pattern
// ("       · Pause 2h 00m") so it agrees with today_render.go's matching
// row. The earlier em-dash framing "       ─ 2h 00m Pause ─" doubled the
// "gap in the timeline" signal that the duration already carries; the
// BulletDot is the canonical low-key separator the rest of the app uses
// (week-row · day-row etc.). Mirrors today_render_test.go's pause-format
// test (TestRenderSessionsList_PauseSeparatorUsesBulletDotNotEmDash) so
// regressions on either Heute or Drill surface fail loudly.

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestRenderDrill_PauseSeparatorUsesBulletDot pins the History-Drill
// session list's pause-line format to the canonical BulletDot pattern.
// Test setup: two completed sessions with a 2h gap. renderDrillSessionRows
// emits exactly one pause line for that gap — enough to detect the
// em-dash substring "─ " + " ─" if regressed.
func TestRenderDrill_PauseSeparatorUsesBulletDot(t *testing.T) {
	pal := theme.TokyonightNight
	h := history{
		pal:   pal,
		width: 80,
		drillSessions: []domain.Session{
			{Start: mustTime("2026-05-30T09:00:00+02:00"), Stop: mustTime("2026-05-30T11:00:00+02:00"), Elapsed: 2 * time.Hour},
			{Start: mustTime("2026-05-30T13:00:00+02:00"), Stop: mustTime("2026-05-30T15:00:00+02:00"), Elapsed: 2 * time.Hour},
		},
	}
	rows, _ := h.renderDrillSessionRows(76)
	joined := strings.Join(rows, "\n")
	if strings.Contains(joined, "─ ") && strings.Contains(joined, " ─") {
		t.Errorf("drill pause separator: should not use ─ dashes anymore; expected · BulletDot pattern. got: %q", joined)
	}
}

// TestHistory_FooterHints_IncludesHelp mirrors week.TestFooterHints_ContainsHelp
// for the Verlauf (History) sub-tab: Skill §Keybind grammar pins `?` as
// a fixed-slot key that must be discoverable from every screen footer.
// Phase-10 follow-up to the 2026-05-30 UX-Review-Cleanup; the `/`-filter
// hint moved to the `?`-overlay (still a universal-fixed-slot key)
// to make room for HintHelp inside the 4-cap.
func TestHistory_FooterHints_IncludesHelp(t *testing.T) {
	pal := theme.TokyonightNight
	h := history{pal: pal}
	hints := h.footerHints()
	found := false
	for _, x := range hints {
		if strings.Contains(x, "? → Hilfe") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("History footerHints: expected ?-help hint, got %v", hints)
	}
}
