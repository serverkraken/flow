package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"time"

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
// marker sits at expected-by-now / Target; expected-by-now = Total − Saldo
// (Saldo is defined as Total − expected). Pct is the logged fill. Both clamp
// to [0,100]; a zero target leaves both at 0.
func burndownBannerVM(rep domain.MonthBurndownReport) components.BurndownVM {
	pct, pace := 0, 0
	if rep.Target > 0 {
		pct = clampPct(int(rep.Total * 100 / rep.Target))
		expected := rep.Total - rep.Saldo
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

func (s *Server) statsData(ctx context.Context, u domain.User) (webui.StatsVM, error) {
	now := s.Clock.Now()

	today, err := s.Stats.Today(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}
	burndown, err := s.Stats.Burndown(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}
	weekDays, err := s.Stats.Week(ctx, u.ID, time.Time{})
	if err != nil {
		return webui.StatsVM{}, err
	}
	set, _, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}

	// Week saldo = Mon–Fri logged − target (weekends excluded, matching Woche).
	var weekLogged, weekTarget time.Duration
	for _, wd := range weekDays {
		if wd.Date.Weekday() == time.Saturday || wd.Date.Weekday() == time.Sunday {
			continue
		}
		weekLogged += wd.Total(now)
		weekTarget += wd.Target
	}
	weekSaldo := weekLogged - weekTarget

	return webui.StatsVM{
		TodaySaldo:    webui.FmtSaldoVerbose(today.Saldo),
		TodayPos:      today.Saldo >= 0,
		TodaySub:      webui.FmtVerbose(today.Logged) + " / " + webui.FmtVerbose(today.Target),
		WeekSaldo:     webui.FmtSaldoVerbose(weekSaldo),
		WeekPos:       weekSaldo >= 0,
		WeekSub:       webui.FmtVerbose(weekLogged) + " / " + webui.FmtVerbose(weekTarget),
		MonthSaldo:    webui.FmtSaldoVerbose(burndown.Saldo),
		MonthPos:      burndown.OnTrack,
		MonthSub:      webui.FmtVerbose(burndown.Total) + " / " + webui.FmtVerbose(burndown.Target),
		Burndown:      burndownBannerVM(burndown),
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}, nil
}

func (s *Server) renderStatsFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	vm, err := s.statsData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsFragment(vm).Render(r.Context(), w)
}

func (s *Server) handleWebStatsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.statsData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebStatsFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderStatsFragment(w, r, u)
}

func (s *Server) handleWebSetTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	defaultMin, err := strconv.Atoi(r.FormValue("defaultTargetMin"))
	if err != nil || defaultMin < 0 {
		http.Error(w, "invalid defaultTargetMin", http.StatusBadRequest)
		return
	}
	// Load existing settings to preserve per-weekday overrides (I1: data-loss fix).
	set, _, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := s.SetTarget.Execute(r.Context(), u.ID, defaultMin, set.WeekdayTargetMin); err != nil {
		if err == domain.ErrInvalidTarget {
			http.Error(w, "invalid target", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderStatsFragment(w, r, u)
}
