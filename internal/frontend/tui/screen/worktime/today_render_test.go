package worktime

// White-box tests for today_render.go's pure (heute, now) → []string
// helpers. Same colour-contract idea as today_badge_test.go (which
// pins the Heute-headline status pill) — here we pin the running-
// session row's colour. Skill §Color semantics: Active (Cyan) marks
// running/live; Success (Green) marks achievement. The two must not
// be conflated across surfaces.

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestRenderSessionsList_RunningSessionUsesActiveNotSuccess pins
// the running-session row's foreground to Sem.Active (Cyan), not
// Sem.Success (Green). Heute's status pill (todayStatusBadge) already
// renders the running state in Sem.Active; the session row below
// must agree. Success is reserved for the achieved state and would
// signal "done" — misleading for a live counter.
//
// Test setup: Active set, no past Sessions, Logged=0, Target=0. With
// target=0, `achieved = total >= target && target > 0` is false, so
// the Headline-side status badge falls into the `running` (not
// `running && achieved`) branch and emits Sem.Active there too —
// meaning the test's negative-Success assertion can't be fooled by
// a stray Success render on the badge line.
func TestRenderSessionsList_RunningSessionUsesActiveNotSuccess(t *testing.T) {
	pal := theme.TokyonightNight
	now := mustTime("2026-05-30T10:30:00+02:00")
	active := mustTime("2026-05-30T09:00:00+02:00")
	h := heute{
		pal:    pal,
		width:  80,
		loaded: true,
		day: domain.Day{
			Active: &active,
			// no past Sessions → running-only state
		},
	}
	rows, _ := h.renderSessionsList(76, now)
	joined := strings.Join(rows, "\n")

	sem := pal.Sem()
	// Active (Cyan) MUST appear — that's the running-line role.
	if !containsFgSGR(joined, sem.Active) {
		t.Errorf("running session: expected Sem.Active (%v) fg SGR, got %q", sem.Active, joined)
	}
	// Success (Green) MUST NOT appear — that's achievement, not live.
	// Note: Sem.Success and Sem.Active have distinct hexes (Green vs.
	// Cyan), so this is a clean separation; with Target=0 the headline
	// path can't introduce Success either.
	if containsFgSGR(joined, sem.Success) {
		t.Errorf("running session: should not carry Sem.Success (%v) fg SGR, got %q", sem.Success, joined)
	}
}

// TestRenderSessionsList_PauseSeparatorUsesBulletDotNotEmDash pins the
// pause-line format between two adjacent sessions to the app-wide
// BulletDot-separator pattern: "       · Pause 2h 00m" instead of the
// previous em-dash-flanked variant "       ─ 2h 00m Pause ─".
//
// The em-dash framing visually doubles the "gap in the timeline" signal
// that the duration already carries; the BulletDot is the canonical
// low-key separator the rest of the app uses (week-row · day-row etc.).
// Test setup: two completed sessions with a 2h gap. The loop in
// renderSessionsList emits exactly one pause line for that gap, which
// is enough to detect the em-dash substring "─ " + " ─" if regressed.
func TestRenderSessionsList_PauseSeparatorUsesBulletDotNotEmDash(t *testing.T) {
	pal := theme.TokyonightNight
	now := mustTime("2026-05-30T16:00:00+02:00")
	h := heute{
		pal: pal, width: 80, loaded: true,
		day: domain.Day{
			Sessions: []domain.Session{
				{Start: mustTime("2026-05-30T09:00:00+02:00"), Stop: mustTime("2026-05-30T11:00:00+02:00"), Elapsed: 2 * time.Hour},
				{Start: mustTime("2026-05-30T13:00:00+02:00"), Stop: mustTime("2026-05-30T15:00:00+02:00"), Elapsed: 2 * time.Hour},
			},
		},
	}
	rows, _ := h.renderSessionsList(76, now)
	joined := strings.Join(rows, "\n")
	if strings.Contains(joined, "─ ") && strings.Contains(joined, " ─") {
		t.Errorf("pause separator: should not use ─ dashes anymore; expected · BulletDot pattern. got: %q", joined)
	}
}
