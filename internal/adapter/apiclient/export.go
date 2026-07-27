package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Export fetches a worktime export. format is "csv"|"json"|"md"; nodeID ""
// means all projects. Returns the raw bytes of the chosen format.
// Auth is injected automatically by the client's RoundTripper transport.
func (c *Client) Export(ctx context.Context, from, to, format, nodeID string) ([]byte, error) {
	q := url.Values{"from": {from}, "to": {to}, "format": {format}}
	if nodeID != "" {
		q.Set("project", nodeID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/export?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apiclient: export status %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// SetNodeRate sets (amount != nil) or clears (amount == nil) a project's
// per-hour rate in minor units.
func (c *Client) SetNodeRate(ctx context.Context, nodeID string, amount *int64, currency string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/nodes/"+nodeID+"/rate",
		map[string]any{"amount": amount, "currency": currency}, nil)
}
