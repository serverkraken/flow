// events.go — Plan E · Task 14 (M7).
//
// SSE endpoint at GET /api/v1/events?stream=ui. The `stream` query is
// decorative for M7 (only "ui" is meaningful); future streams (audit,
// presence) will gate on it. The handler:
//
//   - 401s when no user is in context (defence-in-depth — the cookie-auth
//     group MUST already gate this route)
//   - subscribes to the sse broadcaster scoped to the user's ID
//   - writes the standard SSE headers + an initial ": connected" comment
//     so the browser sees the connection immediately
//   - streams JSON-marshalled events until the request context is done
//     (typically when the browser navigates away or closes the tab)
//
// Reconnect is the browser's job — EventSource auto-reconnects on
// disconnect with a `retry:` interval we don't override. The handler
// returns immediately on context cancel; no goroutine leaks.

package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// NewEvents returns the handler for GET /api/v1/events. The handler is
// long-lived: it holds the response open and streams events until the
// browser disconnects.
func NewEvents(b *sse.Broadcaster) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Hint reverse proxies (nginx, traefik) to disable response
		// buffering for this stream. Without it, nginx will hold our
		// event bytes until the buffer fills, which is exactly the
		// opposite of what we want.
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		ch, cancel := b.Subscribe(u.ID)
		defer cancel()

		// Initial ping so the browser sees a non-empty response body
		// before the first real event. Comment lines (": …") are valid
		// SSE noise that EventSource ignores.
		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, open := <-ch:
				if !open {
					// Broadcaster cancelled the channel (e.g. server
					// shutdown). Nothing further to send.
					return
				}
				data, err := json.Marshal(ev.Data)
				if err != nil {
					slog.Warn(
						"sse: marshal failed; sending empty data",
						slog.String("user_id", u.ID),
						slog.String("event_type", ev.Type),
						slog.String("err", err.Error()),
					)
					data = []byte("null")
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
					// Write failed — client disconnected mid-stream.
					return
				}
				flusher.Flush()
			}
		}
	})
}
