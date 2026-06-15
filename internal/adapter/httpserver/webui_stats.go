package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// fmtMin renders whole minutes as HH:MM.
func fmtMin(m int) string {
	if m < 0 {
		m = -m
		return fmt.Sprintf("-%02d:%02d", m/60, m%60)
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// fmtSaldo renders a duration as a signed saldo string, e.g. "+01:30" or "-00:30".
func fmtSaldo(d time.Duration) string {
	m := int(d / time.Minute)
	if m >= 0 {
		return "+" + fmtMin(m)
	}
	return fmtMin(m)
}

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

func (s *Server) statsData(ctx context.Context, u domain.User, rng string) (webui.StatsData, error) {
	now := s.Clock.Now()

	today, err := s.Stats.Today(ctx, u.ID)
	if err != nil {
		return webui.StatsData{}, err
	}

	burndown, err := s.Stats.Burndown(ctx, u.ID)
	if err != nil {
		return webui.StatsData{}, err
	}

	weekDays, err := s.Stats.Week(ctx, u.ID, time.Time{})
	if err != nil {
		return webui.StatsData{}, err
	}

	if rng == "" {
		rng = "week"
	}
	rangeStats, err := s.Stats.RangeStats(ctx, u.ID, rng)
	if err != nil {
		return webui.StatsData{}, err
	}

	set, _, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.StatsData{}, err
	}

	// Build week rows.
	weekRows := make([]webui.StatsWeekRow, 0, len(weekDays))
	for _, wd := range weekDays {
		logged := int(wd.Total(now) / time.Minute)
		target := int(wd.Target / time.Minute)
		pct := 0
		if target > 0 {
			pct = clampPct(logged * 100 / target)
		}
		isWorkday := wd.Date.Weekday() != time.Saturday && wd.Date.Weekday() != time.Sunday
		weekRows = append(weekRows, webui.StatsWeekRow{
			Date:    wd.Date.Format("Mon 02.01"),
			Logged:  fmtMin(logged),
			Target:  fmtMin(target),
			Pct:     pct,
			IsToday: wd.IsToday,
			Workday: isWorkday,
		})
	}

	// Monthly burndown bar percentage: logged vs target.
	monthPct := 0
	if burndown.Target > 0 {
		monthPct = clampPct(int(burndown.Total * 100 / burndown.Target))
	}

	rangeLabel := "Woche"
	if rng == "month" {
		rangeLabel = "Monat"
	}

	// Range stats.
	rngTotal := int(rangeStats.Total / time.Minute)
	rngAvg := int(rangeStats.Avg / time.Minute)
	rngMax := int(rangeStats.Max / time.Minute)
	rngMin := int(rangeStats.Min / time.Minute)
	rngSaldo := fmtSaldo(rangeStats.Overtime)

	return webui.StatsData{
		User:         u.Username,
		TodayLogged:  fmtMin(int(today.Logged / time.Minute)),
		TodayTarget:  fmtMin(int(today.Target / time.Minute)),
		TodaySaldo:   fmtSaldo(today.Saldo),
		TodayAhead:   today.Saldo >= 0,
		MonthTotal:   fmtMin(int(burndown.Total / time.Minute)),
		MonthTarget:  fmtMin(int(burndown.Target / time.Minute)),
		MonthSaldo:   fmtSaldo(burndown.Saldo),
		MonthOnTrack: burndown.OnTrack,
		MonthPct:     monthPct,
		Week:         weekRows,
		Range: webui.StatsRange{
			Total:      fmtMin(rngTotal),
			Avg:        fmtMin(rngAvg),
			Max:        fmtMin(rngMax),
			Min:        fmtMin(rngMin),
			Workdays:   rangeStats.Workdays,
			Hits:       rangeStats.Hits,
			Streak:     rangeStats.Streak,
			BestStreak: rangeStats.BestStreak,
			Saldo:      rngSaldo,
		},
		RangeLabel:    rangeLabel,
		RangeParam:    rng,
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
	}, nil
}

func (s *Server) renderStatsFragment(w http.ResponseWriter, r *http.Request, u domain.User, rng string) {
	d, err := s.statsData(r.Context(), u, rng)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebStatsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rng := r.URL.Query().Get("range")
	d, err := s.statsData(r.Context(), u, rng)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebStatsFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rng := r.URL.Query().Get("range")
	s.renderStatsFragment(w, r, u, rng)
}

func (s *Server) handleWebSetTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	defaultMin, err := strconv.Atoi(r.FormValue("defaultTargetMin"))
	if err != nil || defaultMin < 0 {
		http.Error(w, "invalid defaultTargetMin", http.StatusBadRequest)
		return
	}
	if err := s.SetTarget.Execute(r.Context(), u.ID, defaultMin, nil); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderStatsFragment(w, r, u, "")
}
