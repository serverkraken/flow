package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// streamClient has no timeout — the events stream is long-lived and ends only
// when the context is cancelled or the server closes.
var streamClient = &http.Client{}

// ClientEvent is a decoded SSE frame: the event name and the small payload.
type ClientEvent struct {
	Type string
	Data map[string]any
}

// Events subscribes to the server SSE stream. The returned channel is closed
// when ctx is cancelled or the connection drops; callers reconnect by calling
// Events again (and should full-refresh their state on reconnect).
func (c *Client) Events(ctx context.Context) (<-chan ClientEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("apiclient: events status %d", res.StatusCode)
	}
	ch := make(chan ClientEvent)
	go func() {
		defer close(ch)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var evType, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				evType = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(line[len("data:"):])
			case line == "":
				if evType == "" {
					continue
				}
				var payload struct {
					Data map[string]any `json:"data"`
				}
				_ = json.Unmarshal([]byte(data), &payload)
				select {
				case ch <- ClientEvent{Type: evType, Data: payload.Data}:
				case <-ctx.Done():
					return
				}
				evType, data = "", ""
			}
		}
	}()
	return ch, nil
}
