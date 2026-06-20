package kindcolor_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestDayOffColor_PerKind(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		k    domain.Kind
		want theme.Color
	}{
		{domain.KindHoliday, sem.Schedule},
		{domain.KindVacation, sem.Highlight},
		{domain.KindSick, sem.Notice},
		{domain.KindFlex, sem.Success},
		{domain.KindSpecial, sem.Warning},
		{domain.KindChildSick, sem.Danger},
		{domain.KindTraining, sem.Info},
	}
	for _, c := range cases {
		if got := kindcolor.DayOffColor(c.k, p); got != c.want {
			t.Errorf("DayOffColor(%q) = %v, want %v", c.k, got, c.want)
		}
	}
}

func TestDayOffColor_UnknownFallsBackToMuted(t *testing.T) {
	t.Parallel()
	p := theme.Default
	if got := kindcolor.DayOffColor(domain.Kind("nonsense"), p); got != p.FgMuted {
		t.Errorf("DayOffColor(unknown) = %v, want FgMuted %v", got, p.FgMuted)
	}
}
