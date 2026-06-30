package httpserver

import (
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// clampPct returns v clamped to [0, 100].
func clampPct(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// burndownBannerVM maps a month burndown report onto the banner VM. The pace
// marker sits at expected-by-now / Target; expected-by-now = TargetTotal − Saldo
// (Saldo is defined as TargetTotal − expected). Pct and pace are both job-scoped
// (TargetTotal vs Target), so private non-counting time does not inflate progress.
// Both clamp to [0,100]; a zero target leaves both at 0.
func burndownBannerVM(rep domain.MonthBurndownReport) components.BurndownVM {
	pct, pace := 0, 0
	if rep.Target > 0 {
		pct = clampPct(int(rep.TargetTotal * 100 / rep.Target))
		expected := rep.TargetTotal - rep.Saldo
		pace = clampPct(int(expected * 100 / rep.Target))
	}
	variant := "under"
	if rep.OnTrack {
		variant = "hit"
	}
	return components.BurndownVM{
		Total:   webui.FmtVerbose(rep.Total),
		Target:  webui.FmtVerbose(rep.Target),
		Pct:     pct,
		PacePct: pace,
		Variant: variant,
		OnTrack: rep.OnTrack,
	}
}
