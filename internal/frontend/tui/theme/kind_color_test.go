package theme_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestKindColor_TokyonightNight pins the Kind→Sem mapping to the
// canonical hue triad. A drift here surfaces as a single-line failure
// listing the offending Kind, the actual hex, and the expected hex —
// avoids guess-work when somebody silently re-points a Sem slot.
func TestKindColor_TokyonightNight(t *testing.T) {
	p := theme.TokyonightNight
	sem := p.Sem()
	cases := []struct {
		kind domain.Kind
		want theme.Color
		name string
	}{
		{domain.KindHoliday, sem.Schedule, "Holiday → Schedule"},
		{domain.KindVacation, sem.Highlight, "Vacation → Highlight"},
		{domain.KindSick, sem.Notice, "Sick → Notice"},
	}
	for _, tt := range cases {
		got := theme.KindColor(p, tt.kind)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestKindColor_UnknownKind ensures any future Kind (or zero-value)
// renders with Fg fallback instead of crashing or returning an empty
// colour that would clear styling on the row.
func TestKindColor_UnknownKind(t *testing.T) {
	p := theme.TokyonightNight
	got := theme.KindColor(p, domain.Kind("nope"))
	if got != p.Fg {
		t.Errorf("unknown kind: got %v, want Fg %v", got, p.Fg)
	}
}

// TestKindColor_CrossSurfaceWithKindStatusColor verifies the in-app
// KindColor and the tmux-side domain.KindStatusColor land on identical
// hex values. The two functions are intentionally not collapsed (one
// returns lipgloss colour, the other a hex string for tmux markers) —
// this test is the safety net that keeps them in lockstep.
func TestKindColor_CrossSurfaceWithKindStatusColor(t *testing.T) {
	p := theme.TokyonightNight
	statusPal := theme.StatusPaletteFor(p)
	for _, k := range []domain.Kind{domain.KindHoliday, domain.KindVacation, domain.KindSick} {
		inApp := string(theme.KindColor(p, k).(theme.Color))
		tmux := domain.KindStatusColor(k, statusPal)
		if inApp != tmux {
			t.Errorf("kind %v: in-app %q vs tmux %q", k, inApp, tmux)
		}
	}
}
