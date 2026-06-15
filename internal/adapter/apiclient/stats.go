package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// Today mirrors the server's todayDTO wire shape.
type Today struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	SaldoMin  int    `json:"saldoMin"`
	Running   bool   `json:"running"`
}

// WeekDay mirrors one entry in the server's weekDTO wire shape.
type WeekDay struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	IsToday   bool   `json:"isToday"`
	Workday   bool   `json:"workday"`
}

// Stats mirrors the server's statsDTO wire shape.
type Stats struct {
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

// Burndown mirrors the server's burndownDTO wire shape.
type Burndown struct {
	TotalMin    int  `json:"totalMin"`
	TargetMin   int  `json:"targetMin"`
	SaldoMin    int  `json:"saldoMin"`
	OnTrack     bool `json:"onTrack"`
	WorkdaysAll int  `json:"workdaysAll"`
	WorkdaysDue int  `json:"workdaysDue"`
}

func (c *Client) GetToday(ctx context.Context) (Today, error) {
	var t Today
	err := c.do(ctx, http.MethodGet, "/api/v1/today", nil, &t)
	return t, err
}

func (c *Client) GetWeek(ctx context.Context, ref string) ([]WeekDay, error) {
	path := "/api/v1/week"
	if ref != "" {
		path += "?" + url.Values{"ref": {ref}}.Encode()
	}
	var out []WeekDay
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) GetStats(ctx context.Context, rng string) (Stats, error) {
	var s Stats
	err := c.do(ctx, http.MethodGet, "/api/v1/stats?"+url.Values{"range": {rng}}.Encode(), nil, &s)
	return s, err
}

func (c *Client) GetBurndown(ctx context.Context) (Burndown, error) {
	var b Burndown
	err := c.do(ctx, http.MethodGet, "/api/v1/burndown", nil, &b)
	return b, err
}
