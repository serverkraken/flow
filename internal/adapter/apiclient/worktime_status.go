package apiclient

import (
	"context"
	"net/http"
)

// WorktimeStatus mirrors the server's worktimeStatusDTO (tmux status segment).
type WorktimeStatus struct {
	Date            string      `json:"date"`
	LoggedMin       int         `json:"loggedMin"`
	TargetMin       int         `json:"targetMin"`
	Running         bool        `json:"running"`
	ActiveSessionID string      `json:"activeSessionId"`
	ActiveStart     string      `json:"activeStart"`
	ActiveNodeID    *string     `json:"activeNodeId"`
	DayOff          *WSDayOff   `json:"dayOff"`
	Week            []WSWeekDay `json:"week"`
	Streak          int         `json:"streak"`
	Burndown        WSBurndown  `json:"burndown"`
}

type WSDayOff struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type WSWeekDay struct {
	Date       string  `json:"date"`
	LoggedMin  int     `json:"loggedMin"`
	TargetMin  int     `json:"targetMin"`
	Workday    bool    `json:"workday"`
	IsToday    bool    `json:"isToday"`
	DayOffKind *string `json:"dayOffKind"`
}

type WSBurndown struct {
	SaldoMin  int `json:"saldoMin"`
	TargetMin int `json:"targetMin"`
}

// GetWorktimeStatus fetches the aggregated tmux status snapshot. The caller sets
// a short deadline via context (the status tick uses ~2s); do() honours it.
func (c *Client) GetWorktimeStatus(ctx context.Context) (WorktimeStatus, error) {
	var out WorktimeStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/worktime/status", nil, &out)
	return out, err
}
