package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// todayDTO is the wire shape for the today summary.
type todayDTO struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	SaldoMin  int    `json:"saldoMin"`
	Running   bool   `json:"running"`
}

// weekDayDTO is one day in the week view.
type weekDayDTO struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	IsToday   bool   `json:"isToday"`
	Workday   bool   `json:"workday"`
}

// minutes converts a Duration to whole minutes for the wire.
func minutes(d time.Duration) int { return int(d / time.Minute) }

// statsDTO is the wire shape for the range stats endpoint.
type statsDTO struct {
	Days             int `json:"days"`
	DaysWithSessions int `json:"daysWithSessions"`
	Workdays         int `json:"workdays"`
	TotalMin         int `json:"totalMin"`
	AvgMin           int `json:"avgMin"`
	MaxMin           int `json:"maxMin"`
	MinMin           int `json:"minMin"`
	Hits             int `json:"hits"`
	Streak           int `json:"streak"`
	BestStreak       int `json:"bestStreak"`
	OvertimeMin      int `json:"overtimeMin"`
	TargetTotalMin   int `json:"targetTotalMin"`
}

// burndownDTO is the wire shape for the monthly burndown.
type burndownDTO struct {
	TotalMin       int  `json:"totalMin"`
	TargetMin      int  `json:"targetMin"`
	SaldoMin       int  `json:"saldoMin"`
	OnTrack        bool `json:"onTrack"`
	WorkdaysAll    int  `json:"workdaysAll"`
	WorkdaysDue    int  `json:"workdaysDue"`
	TargetTotalMin int  `json:"targetTotalMin"`
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	sum, err := s.Stats.Today(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, todayDTO{
		Date:      sum.Date.Format(dayFmt),
		LoggedMin: minutes(sum.Logged),
		TargetMin: minutes(sum.Target),
		SaldoMin:  minutes(sum.Saldo),
		Running:   sum.Running,
	})
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var ref time.Time
	if q := r.URL.Query().Get("ref"); q != "" {
		t, err := time.ParseInLocation(dayFmt, q, time.Local)
		if err != nil {
			http.Error(w, "ref must be yyyy-mm-dd", http.StatusBadRequest)
			return
		}
		ref = t
	}
	days, err := s.Stats.Week(r.Context(), u.ID, ref)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	now := s.Clock.Now()
	out := make([]weekDayDTO, 0, len(days))
	for _, d := range days {
		out = append(out, weekDayDTO{
			Date: d.Date.Format(dayFmt), LoggedMin: minutes(d.Total(now)),
			TargetMin: minutes(d.Target), IsToday: d.IsToday,
			Workday: d.Date.Weekday() != time.Saturday && d.Date.Weekday() != time.Sunday,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rng := r.URL.Query().Get("range")
	st, err := s.Stats.RangeStats(r.Context(), u.ID, rng)
	if errors.Is(err, domain.ErrInvalidRange) {
		http.Error(w, "invalid range: use 'week' or 'month'", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, statsDTO{
		Days:             st.Days,
		DaysWithSessions: st.DaysWithSessions,
		Workdays:         st.Workdays,
		TotalMin:         minutes(st.Total),
		AvgMin:           minutes(st.Avg),
		MaxMin:           minutes(st.Max),
		MinMin:           minutes(st.Min),
		Hits:             st.Hits,
		Streak:           st.Streak,
		BestStreak:       st.BestStreak,
		OvertimeMin:      minutes(st.Overtime),
		TargetTotalMin:   minutes(st.TargetTotal),
	})
}

// nodeRollupDTO is the wire shape for the subtree worktime rollup.
// Work*Min is the subset of *Min that counts toward the Soll (effective
// CountsTowardTarget flag = Work); Privat is Total-Work / Week-WorkWeek /
// Month-WorkMonth, derived by the consumer.
type nodeRollupDTO struct {
	TotalMin     int `json:"totalMin"`
	WeekMin      int `json:"weekMin"`
	MonthMin     int `json:"monthMin"`
	WorkTotalMin int `json:"workTotalMin"`
	WorkWeekMin  int `json:"workWeekMin"`
	WorkMonthMin int `json:"workMonthMin"`
	// YearMin/PrevYearToDateMin feed Screen 02's year tile; both are additive,
	// so apiclient.NodeRollup (three fields) keeps decoding unchanged.
	YearMin           int `json:"yearMin"`
	PrevYearToDateMin int `json:"prevYearToDateMin"`
}

func (s *Server) handleNodeStats(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	roll, err := s.Stats.NodeStats(r.Context(), u.ID, id)
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, nodeRollupDTO{
			TotalMin:          minutes(roll.Total),
			WeekMin:           minutes(roll.Week),
			MonthMin:          minutes(roll.Month),
			WorkTotalMin:      minutes(roll.WorkTotal),
			WorkWeekMin:       minutes(roll.WorkWeek),
			WorkMonthMin:      minutes(roll.WorkMonth),
			YearMin:           minutes(roll.Year),
			PrevYearToDateMin: minutes(roll.PrevYearToDate),
		})
	}
}

func (s *Server) handleBurndown(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rep, err := s.Stats.Burndown(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, burndownDTO{
		TotalMin:       minutes(rep.Total),
		TargetMin:      minutes(rep.Target),
		SaldoMin:       minutes(rep.Saldo),
		OnTrack:        rep.OnTrack,
		WorkdaysAll:    rep.WorkdaysAll,
		WorkdaysDue:    rep.WorkdaysDue,
		TargetTotalMin: minutes(rep.TargetTotal),
	})
}
