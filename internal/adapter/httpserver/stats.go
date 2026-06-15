package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
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
}

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
}

// burndownDTO is the wire shape for the monthly burndown.
type burndownDTO struct {
	TotalMin    int  `json:"totalMin"`
	TargetMin   int  `json:"targetMin"`
	SaldoMin    int  `json:"saldoMin"`
	OnTrack     bool `json:"onTrack"`
	WorkdaysAll int  `json:"workdaysAll"`
	WorkdaysDue int  `json:"workdaysDue"`
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
		LoggedMin: int(sum.Logged / time.Minute),
		TargetMin: int(sum.Target / time.Minute),
		SaldoMin:  int(sum.Saldo / time.Minute),
		Running:   sum.Running,
	})
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var ref time.Time
	if q := r.URL.Query().Get("ref"); q != "" {
		if t, err := time.ParseInLocation(dayFmt, q, time.Local); err == nil {
			ref = t
		}
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
			Date:      d.Date.Format(dayFmt),
			LoggedMin: int(d.Total(now) / time.Minute),
			TargetMin: int(d.Target / time.Minute),
			IsToday:   d.IsToday,
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
		TotalMin:         int(st.Total / time.Minute),
		AvgMin:           int(st.Avg / time.Minute),
		MaxMin:           int(st.Max / time.Minute),
		MinMin:           int(st.Min / time.Minute),
		Hits:             st.Hits,
		Streak:           st.Streak,
		BestStreak:       st.BestStreak,
		OvertimeMin:      int(st.Overtime / time.Minute),
	})
}

func (s *Server) handleBurndown(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rep, err := s.Stats.Burndown(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, burndownDTO{
		TotalMin:    int(rep.Total / time.Minute),
		TargetMin:   int(rep.Target / time.Minute),
		SaldoMin:    int(rep.Saldo / time.Minute),
		OnTrack:     rep.OnTrack,
		WorkdaysAll: rep.WorkdaysAll,
		WorkdaysDue: rep.WorkdaysDue,
	})
}
