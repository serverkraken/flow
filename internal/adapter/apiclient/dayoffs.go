package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DayOff is the client-side view of a merged day-off (manual or computed
// holiday). Mirrors the server's dayOffDTO wire shape.
type DayOff struct {
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	TargetMin int    `json:"targetMin"`
	Holiday   bool   `json:"holiday"`
}

func (c *Client) AddDayOffs(ctx context.Context, from, to, kind, label string, targetMin int, skipWeekends bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/dayoffs", map[string]any{
		"from": from, "to": to, "kind": kind, "label": label,
		"targetMin": targetMin, "skipWeekends": skipWeekends,
	}, nil)
}

func (c *Client) ListDayOffs(ctx context.Context, from, to string) ([]DayOff, error) {
	q := url.Values{"from": {from}, "to": {to}}
	var out []DayOff
	err := c.do(ctx, http.MethodGet, "/api/v1/dayoffs?"+q.Encode(), nil, &out)
	return out, err
}

func (c *Client) DeleteDayOff(ctx context.Context, day string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/dayoffs/"+day, nil, nil)
}

// Settings mirrors the server settingsDTO.
type Settings struct {
	Bundesland string   `json:"bundesland"`
	FeedURLs   []string `json:"feedUrls"`
}

func (c *Client) GetSettings(ctx context.Context) (Settings, error) {
	var s Settings
	err := c.do(ctx, http.MethodGet, "/api/v1/settings", nil, &s)
	return s, err
}

func (c *Client) SetBundesland(ctx context.Context, land string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/settings/bundesland", map[string]any{"bundesland": land}, nil)
}

// RegenIcsToken mints a new feed token and returns its absolute URL.
func (c *Client) RegenIcsToken(ctx context.Context) (string, error) {
	var out struct {
		Token   string `json:"token"`
		FeedURL string `json:"feedUrl"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/ics-token/regenerate", nil, &out)
	return out.FeedURL, err
}
