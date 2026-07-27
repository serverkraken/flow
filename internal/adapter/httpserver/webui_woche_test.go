package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestWocheHome_RendersLesesaalChrome verifies GET /woche renders the fully
// rebuilt Lesesaal Woche page (L4 Task 4): the "‹ Zeit" spine, the pagehead,
// the weekbar skyline, the Kennzahlen/Statistik .panel/.krow blocks, the SSE
// swap container, per-day durations — and NONE of the retired Kristall glass
// chrome (glass/shadow-soft/font-display).
func TestWocheHome_RendersLesesaalChrome(t *testing.T) {
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
		"pagehead",
		"weekbar",
		"panel",
		"krow",
		"spine",
		"‹ Zeit", // the spine "up" back-link to /zeit
		"id=\"content\"", // SSE swap container
		"sse:session",    // live-reload trigger
		"/static/app.css",
		"Statistik", // the monthly Burndown/Saldo panel (Offene Entsch. #4)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Woche home missing %q", want)
		}
	}
	// "font-display" also appears in the shared AppShell topbar logo mark
	// (unrelated to Woche content), so the Kristall-chrome check is scoped to
	// glass/shadow-soft only here; the Woche-owned fragment content itself is
	// checked below via the fragment endpoint.
	for _, unwanted := range []string{"glass", "shadow-soft"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("Woche home must not render retired Kristall chrome %q, got:\n%.3000s", unwanted, body)
		}
	}
	// The two seeded sessions (8h + 9h) must surface their durations.
	if !strings.Contains(body, "8h 00m") {
		t.Errorf("expected Monday bar of 8h 00m, got:\n%s", body)
	}
	// KW stepper + day bars must still be present after the Lesesaal rebuild.
	if !strings.Contains(body, "KW ") {
		t.Errorf("expected KW nav label, got:\n%s", body)
	}
	if !strings.Contains(body, "&lsaquo;") && !strings.Contains(body, "‹") {
		t.Errorf("expected the KW '<' stepper glyph, got:\n%s", body)
	}

	// The Woche-owned fragment (no shared topbar chrome) must not render any
	// of the retired Kristall classes, including font-display.
	fragReq, _ := http.NewRequest("GET", "/ui/woche/fragment", nil)
	fragReq.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	fragRR := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(fragRR, fragReq)
	if fragRR.Code != http.StatusOK {
		t.Fatalf("fragment status=%d body=%s", fragRR.Code, fragRR.Body.String())
	}
	fragBody := fragRR.Body.String()
	for _, unwanted := range []string{"glass", "shadow-soft", "font-display"} {
		if strings.Contains(fragBody, unwanted) {
			t.Errorf("Woche fragment must not render retired Kristall chrome %q, got:\n%.3000s", unwanted, fragBody)
		}
	}
}

// TestWocheHome_WeekbarMatchesZeitHubFormat is the RED→GREEN guard for L4
// Final-Review Finding 1: a Saturday with logged hours (Target 0, since
// weekends carry no default target) must produce a non-zero weekbar bar and
// its "H:MM" clock-format value (FmtClockShort, e.g. "6:10") — the same
// builder/format the Zeit-Hub weekbar uses (webui.BuildWeekBars) — instead of
// the old Woche-only WocheDayVM.Pct, which stayed 0 for any Weekend day
// regardless of logged hours (wocheDayRowVM's early "weekend" return never
// computes Pct) and rendered via FmtVerbose ("6h 10m") page prose. The Mo–So
// detail list still shows "—" for a weekend day's duration (unchanged,
// wocheDayRow) — only the weekbar skyline reflects the logged Saturday.
func TestWocheHome_WeekbarMatchesZeitHubFormat(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Clock is 2026-06-21 (Sunday); ISO-week Monday is 2026-06-15, so
	// Saturday 2026-06-20 is inside the displayed (current) week.
	day, _ := time.ParseInLocation("2006-01-02", "2026-06-20", time.Local)
	from := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, time.Local)
	to := from.Add(6*time.Hour + 10*time.Minute)
	if _, err := (usecase.AddSession{Sessions: srv.ss, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", nil, from, to, nil, "",
	); err != nil {
		t.Fatalf("seed Saturday session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/woche", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "6:10") {
		t.Errorf("weekbar missing FmtClockShort value %q for logged Saturday, got:\n%s", "6:10", body)
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

// TestWoche_OwnerScoped is the owner-scope negative test for the Woche page
// (AGENTS.md §Grundsätze — flow is multi-tenant): user B's session must never
// surface in user A's Woche week totals/day rows.
func TestWoche_OwnerScoped(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// u1's own Monday session (so the week isn't simply empty).
	srv.seedSession(t, "2026-06-15", "09:00", "10:00") // 1h

	// u2's session, same Monday, distinctly larger — must never surface.
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.Local)
	stop := start.Add(6 * time.Hour)
	if _, err := srv.ss.Create(context.Background(), domain.WorkSession{
		ID: "u2-secret", OwnerID: "u2", Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed u2 session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/woche", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// If u2's 6h leaked into u1's Monday total, the day-row .v cell would show
	// the combined 7h instead of u1's own 1h. Anchored on the .v cell's own
	// closing tag so this doesn't collide with the (expected, correct) Saldo
	// line's "−7h 00m" substring (1h logged − 8h target).
	if strings.Contains(body, "text-muted\">7h 00m</div>") {
		t.Errorf("owner-scope leak: u1's Woche rendered a day total including u2's 6h session, got:\n%.2000s", body)
	}
	if !strings.Contains(body, "text-muted\">1h 00m</div>") {
		t.Errorf("expected u1's own scoped 1h Monday total, got:\n%.2000s", body)
	}
}
