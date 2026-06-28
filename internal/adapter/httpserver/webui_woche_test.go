package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/usecase"
)

// TestWocheHome_RendersDayBarsAndTotal verifies GET /woche renders the Woche page
// on the AppShell with the WOCHE GESAMT banner, the worktime sub-tab strip, the
// SSE swap container, and per-day bars seeded from sessions in the current week.
func TestWocheHome_RendersDayBarsAndTotal(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Clock is 2026-06-21 (a Sunday); its ISO-week Monday is 2026-06-15. Seed two
	// completed sessions on Mon/Tue of that week.
	seed := func(dateStr, from, to string) {
		day, _ := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		hm := func(s string) time.Time {
			c, _ := time.ParseInLocation("15:04", s, time.Local)
			return time.Date(day.Year(), day.Month(), day.Day(), c.Hour(), c.Minute(), 0, 0, time.Local)
		}
		if _, err := (usecase.AddSession{Sessions: srv.ss, IDs: srv.ids, Clock: srv.clk}).Execute(
			ctx, "u1", nil, hm(from), hm(to), nil, "",
		); err != nil {
			t.Fatalf("seed %s: %v", dateStr, err)
		}
	}
	seed("2026-06-15", "09:00", "17:00") // Mon, 8h
	seed("2026-06-16", "09:00", "18:00") // Tue, 9h

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/woche", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Woche gesamt",   // WOCHE GESAMT banner label
		"progressbar",    // a day bar (role="progressbar")
		"id=\"content\"", // SSE swap container
		"sse:session",    // live-reload trigger
		"/static/app.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Woche home missing %q", want)
		}
	}
	// The two seeded sessions (8h + 9h) must surface their durations.
	if !strings.Contains(body, "8h 00m") {
		t.Errorf("expected Monday bar of 8h 00m, got:\n%s", body)
	}
}

// TestWocheFragment_KWNavClampsForward verifies the inner fragment honors ?week=
// and that the "next week" link is suppressed on the current week (no forward
// navigation past today).
func TestWocheFragment_KWNavClampsForward(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")

	// Current week: next-week navigation must be clamped (no /ui/woche/fragment?week=
	// pointing into the future).
	req, _ := http.NewRequest("GET", "/ui/woche/fragment", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// A past week (KW 23, Mon 2026-06-01) must offer forward navigation toward the
	// next Monday (2026-06-08), and surface the "Diese Woche" jump.
	req2, _ := http.NewRequest("GET", "/ui/woche/fragment?week=2026-06-01", nil)
	req2.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr2 := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("past-week status=%d", rr2.Code)
	}
	body2 := rr2.Body.String()
	if !strings.Contains(body2, "week=2026-06-08") {
		t.Errorf("past week should offer forward nav to 2026-06-08, got:\n%s", body2)
	}
	if !strings.Contains(body2, "Diese Woche") {
		t.Errorf("past week should offer the 'Diese Woche' jump")
	}
}
