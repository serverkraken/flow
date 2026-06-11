package handlers

// events_test.go — Plan E · Task 14 (M7).
//
// Covers the SSE handler in isolation (no router) plus a router-level
// smoke that asserts the route is mounted under the cookie-auth group.

import (
	"bufio"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// — direct ServeHTTP tests ---------------------------------------------------

func TestEvents_NoUser_401(t *testing.T) {
	t.Parallel()
	b := sse.New()
	h := NewEvents(b)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/events?stream=ui", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

func TestEvents_SetsSSEHeaders_AndInitialPing(t *testing.T) {
	t.Parallel()
	b := sse.New()
	h := NewEvents(b)

	// Cancel the request context after a short delay so the handler
	// returns instead of blocking forever.
	ctx, cancel := context.WithCancel(httpserver.WithUser(context.Background(), domain.User{ID: "u1"}))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/events?stream=ui", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, r)
		close(done)
	}()

	// Give the handler a moment to write headers + the initial ping.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want text/event-stream", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control: got %q, want no-cache", got)
	}
	if got := rr.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering: got %q, want no", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, ": connected") {
		t.Errorf("initial ping missing — body=%q", body)
	}
}

// TestEvents_StreamsPublishedEvent uses an httptest.Server so the
// handler sees a real http.Flusher (httptest.ResponseRecorder doesn't
// implement Flusher; the test above only verifies the prelude header
// path which works regardless). We boot a tiny chi router with just
// this handler under a stub middleware that injects the user.
func TestEvents_StreamsPublishedEvent(t *testing.T) {
	t.Parallel()
	b := sse.New()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/events", injectUser(domain.User{ID: "u-stream"}, NewEvents(b)))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?stream=ui", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want text/event-stream", got)
	}

	br := bufio.NewReader(resp.Body)

	// Drain the initial ": connected" comment + blank line.
	if _, err := readUntilBlank(br); err != nil {
		t.Fatalf("read initial ping: %v", err)
	}

	// Publish from another goroutine — must give the subscriber's
	// goroutine time to call Subscribe before we publish. We poll the
	// broadcaster's subscriber count indirectly by retrying.
	go func() {
		// Brief delay so the SSE goroutine completes Subscribe + the
		// initial ping flush.
		time.Sleep(50 * time.Millisecond)
		b.Publish("u-stream", sse.Event{Type: "session.started", Data: map[string]string{"id": "s1"}})
	}()

	frame, err := readFrame(br, 2*time.Second)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !strings.Contains(frame, "event: session.started") {
		t.Errorf("frame missing event type line: %q", frame)
	}
	if !strings.Contains(frame, `data: {"id":"s1"}`) {
		t.Errorf("frame missing data line: %q", frame)
	}
}

// — router-level wiring smoke ------------------------------------------------
//
// Reuses encodeTestSession from sessioncookie_test.go (Task 13 cleanup).
// Boots the full server, hits /api/v1/events with a real signed cookie,
// and asserts we get the SSE prelude — proving the route is mounted
// under the cookie group and the broadcaster is wired in.

func TestRouter_GET_Events_Streams(t *testing.T) {
	t.Parallel()
	users := pgstore.NewUsers(pgWebUIStore)

	b := sse.New()
	webUI := &httpserver.WebUIHandlers{
		Events: NewEvents(b),
	}

	hashKey, _ := hex.DecodeString(strings.Repeat("33", 32))
	blockKey, _ := hex.DecodeString(strings.Repeat("44", 16))
	sess := httpserver.NewSession(hashKey, blockKey)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:     fakeProvider{id: ports.Identity{Sub: "u-sse-router", Email: "sse@example", Name: "sse"}},
		Access:       fakeAccess{ok: true},
		Session:      sess,
		Users:        users,
		WebUI:        webUI,
		BaseURL:      "http://localhost:0",
		OIDCClientID: "test-client",
		OIDCSecret:   "test-secret",
		Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: false},
		Ready:        func() error { return nil },
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	cookieVal := encodeTestSession(t, sess, "flow_session", "u-sse-router", "sse@example", "sse", time.Hour)
	jar, _ := cookiejar.New(nil)
	tsURL, _ := url.Parse(ts.URL)
	jar.SetCookies(tsURL, []*http.Cookie{{Name: "flow_session", Value: cookieVal}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/events?stream=ui", nil)
	for _, c := range jar.Cookies(tsURL) {
		req.AddCookie(c)
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// A redirect means the cookie-auth middleware kicked us to
			// /auth/landing — fail fast so the wiring regression is loud.
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200 — route not mounted or middleware kicked us", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want text/event-stream", got)
	}

	br := bufio.NewReader(resp.Body)
	prelude, err := readUntilBlank(br)
	if err != nil {
		t.Fatalf("read prelude: %v", err)
	}
	if !strings.Contains(prelude, ": connected") {
		t.Errorf("prelude missing ': connected' comment: %q", prelude)
	}
}

// — helpers ------------------------------------------------------------------

// injectUser is a minimal middleware that puts the supplied user into
// the request context. Mirrors what BrowserAuthMiddleware does, without
// the cookie path.
func injectUser(u domain.User, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := httpserver.WithUser(r.Context(), u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// readUntilBlank reads lines from br until it hits a blank line (an SSE
// frame terminator). Returns everything read so the test can assert on
// the prelude content.
func readUntilBlank(br *bufio.Reader) (string, error) {
	var out strings.Builder
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			out.WriteString(line)
		}
		if err != nil {
			if err == io.EOF {
				return out.String(), nil
			}
			return out.String(), err
		}
		if line == "\n" || line == "\r\n" {
			return out.String(), nil
		}
	}
}

// readFrame reads one SSE frame (lines until a blank terminator) with a
// timeout. Polls in a tight loop because bufio.Reader has no native
// deadline support; the upstream resp.Body close on context cancel
// would unblock a stuck read.
func readFrame(br *bufio.Reader, timeout time.Duration) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := readUntilBlank(br)
		ch <- result{s, err}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(timeout):
		return "", io.ErrUnexpectedEOF
	}
}
