// internal/webui/handlers/events_heartbeat_test.go
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/sse"
)

func TestEvents_HeartbeatComment(t *testing.T) {
	old := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	defer func() { heartbeatInterval = old }()

	b := sse.New()
	h := NewEvents(b)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 120*time.Millisecond)
	defer cancel()
	req = req.WithContext(httpserver.WithUser(ctx, domain.User{ID: "hb-user"}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), ": hb") {
		t.Errorf("heartbeat comment missing in stream: %q", rec.Body.String())
	}
}
