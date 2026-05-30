package worktime

// White-box tests for today_render.go's pure (heute, now) → []string
// helpers. Same colour-contract idea as today_badge_test.go (which
// pins the Heute-headline status pill) — here we pin the running-
// session row's colour. Skill §Color semantics: Active (Cyan) marks
// running/live; Success (Green) marks achievement. The two must not
// be conflated across surfaces.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// mustParseTime parses RFC3339 or panics — test-only helper for
// constructing fixed clocks. Local to this file to keep blast-radius
// small; if a second worktime test file needs the same shape it can
// be promoted to helpers_test.go.
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// containsFgSGR mirrors theme.builders_test.containsForeground —
// lipgloss v2 emits truecolor as `38;2;R;G;B`, the `#rrggbb` form is
// what `%v` prints but never appears literally in the rendered
// output. Decode hex → RGB and look for the SGR triplet instead.
func containsFgSGR(out string, c theme.Color) bool {
	hex := strings.TrimPrefix(fmt.Sprintf("%v", c), "#")
	if len(hex) != 6 {
		return false
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return false
	}
	return strings.Contains(out, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
}

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
	now := mustParseTime("2026-05-30T10:30:00+02:00")
	active := mustParseTime("2026-05-30T09:00:00+02:00")
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
