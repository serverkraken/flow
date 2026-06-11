package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Events konsumiert /api/v1/events (SSE): "changed"-Events invalidieren die
// Resource-Caches und wecken Changed() — die TUI refetcht dann. Reißt der
// Stream, gilt Reconnect-mit-Refetch (Spec §7): nach jedem (Re-)Connect wird
// einmal pauschal invalidiert. Zusätzlich weckt ein Fallback-Poll alle 45 s.
type Events struct {
	c          *Client
	invalidate func(resource string) // verdrahtet in main.go über alle Adapter
	changed    chan struct{}
	stop       context.CancelFunc
	done       chan struct{}
}

func NewEvents(c *Client, invalidate func(string)) *Events {
	return &Events{c: c, invalidate: invalidate, changed: make(chan struct{}, 1), done: make(chan struct{})}
}

// Changed weckt die UI (coalesced); nach jedem Wecken: neu laden.
func (e *Events) Changed() <-chan struct{} { return e.changed }

func (e *Events) notify() {
	select {
	case e.changed <- struct{}{}:
	default:
	}
}

func (e *Events) invalidateAll() {
	for _, r := range []string{"worktime", "projects", "documents", "dayoffs"} {
		e.invalidate(r)
	}
}

func (e *Events) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	e.stop = cancel
	go e.loop(ctx)
}

func (e *Events) Stop() {
	if e.stop != nil {
		e.stop()
	}
	<-e.done
}

func (e *Events) loop(ctx context.Context) {
	defer close(e.done)
	bo := Backoff{}
	attempt := 0
	poll := time.NewTicker(45 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			e.invalidateAll()
			e.notify()
			continue
		default:
		}
		err := e.stream(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Debug("sse: stream ended", "err", err)
		}
		attempt++
		select {
		case <-ctx.Done():
			return
		case <-time.After(bo.For(attempt)):
		}
	}
}

// stream verbindet einmal und liest bis zum Fehler. Erfolgreicher Connect
// (HTTP 200) setzt attempt-Reset beim Aufrufer voraus — bewusst simpel:
// invalidateAll nach Connect deckt verpasste Events ab.
func (e *Events) stream(ctx context.Context) error {
	tok, err := e.c.bearer()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.c.base+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Flow-Client-Version", e.c.version)
	httpc := &http.Client{Timeout: 0} // Stream: kein Gesamt-Timeout
	resp, err := httpc.Do(req)
	if err != nil {
		e.c.status.setOffline()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode}
	}
	e.c.status.setOnline(e.c.base)
	e.invalidateAll() // Reconnect-Refetch
	e.notify()
	sc := bufio.NewScanner(resp.Body)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if event == "changed" {
				var d struct {
					Resource string `json:"resource"`
				}
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &d) == nil {
					e.invalidate(d.Resource)
					e.notify()
				}
			}
			event = ""
		}
	}
	return sc.Err()
}
