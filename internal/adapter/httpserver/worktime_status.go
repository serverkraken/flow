package httpserver

import (
	"net/http"
	"time"
)

type worktimeStatusDTO struct {
	Date            string `json:"date"`
	LoggedMin       int    `json:"loggedMin"`
	TargetMin       int    `json:"targetMin"`
	Running         bool   `json:"running"`
	ActiveSessionID string `json:"activeSessionId,omitempty"`
	ActiveStart     string `json:"activeStart,omitempty"`
	// omitempty: an unbooked running session omits activeNodeId (the wire example
	// shows it as null). Absent ≡ null ≡ unbooked — apiclient.WorktimeStatus
	// decodes both to a nil *string identically (Finding C7, consumer-verified).
	ActiveNodeID *string        `json:"activeNodeId,omitempty"`
	DayOff       *wsDayOffDTO   `json:"dayOff"`
	Week         []wsWeekDayDTO `json:"week"`
	Streak       int            `json:"streak"`
	Burndown     wsBurndownDTO  `json:"burndown"`
}

type wsDayOffDTO struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type wsWeekDayDTO struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	// Workday is part of the public wire contract (Spec §1) for future consumers
	// (e.g. a web client). The tmux CLI itself re-derives the weekday from Date,
	// so this field is unread on that path — kept for contract completeness.
	Workday    bool    `json:"workday"`
	IsToday    bool    `json:"isToday"`
	DayOffKind *string `json:"dayOffKind"`
}

type wsBurndownDTO struct {
	SaldoMin  int `json:"saldoMin"`
	TargetMin int `json:"targetMin"`
}

func (s *Server) handleWorktimeStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	res, err := s.WorktimeStatus.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	dto := worktimeStatusDTO{
		Date:      res.Date.Format(dayFmt),
		LoggedMin: minutes(res.Logged),
		TargetMin: minutes(res.Target),
		Running:   res.Running,
		Streak:    res.Streak,
		Burndown:  wsBurndownDTO{SaldoMin: minutes(res.Burndown.Saldo), TargetMin: minutes(res.Burndown.Target)},
	}
	if res.Running {
		dto.ActiveSessionID = res.ActiveID
		dto.ActiveStart = res.ActiveStart.Format(time.RFC3339)
		dto.ActiveNodeID = res.ActiveNodeID
	}
	if res.DayOff != nil {
		dto.DayOff = &wsDayOffDTO{Kind: string(res.DayOff.Kind), Label: res.DayOff.Label}
	}
	dto.Week = make([]wsWeekDayDTO, 0, len(res.Week))
	for _, d := range res.Week {
		row := wsWeekDayDTO{
			Date: d.Date.Format(dayFmt), LoggedMin: minutes(d.Logged), TargetMin: minutes(d.Target),
			Workday: d.Date.Weekday() != time.Saturday && d.Date.Weekday() != time.Sunday,
			IsToday: d.IsToday,
		}
		if d.DayOffKind != "" {
			k := string(d.DayOffKind)
			row.DayOffKind = &k
		}
		dto.Week = append(dto.Week, row)
	}
	writeJSON(w, http.StatusOK, dto)
}
