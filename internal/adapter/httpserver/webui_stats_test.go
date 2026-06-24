package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebStatsServer wires the stats web handlers behind cookie auth, with a
// pre-seeded user "u1" whose session cookie the test forges via the codec.
// It also returns the shared FakeUserSettingsStore so tests can inspect stored state.
func newWebStatsServer(t *testing.T) (*httpserver.Server, *websession.Codec, *testutil.FakeUserSettingsStore) {
	srv, codec, settings, _ := newWebStatsServerFull(t)
	return srv, codec, settings
}

// newWebStatsServerFull is like newWebStatsServer but also returns the
// FakeSessionStore so callers can pre-seed sessions.
func newWebStatsServerFull(t *testing.T) (*httpserver.Server, *websession.Codec, *testutil.FakeUserSettingsStore, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	sessions := testutil.NewFakeSessionStore()
	dos := testutil.NewFakeDayOffStore()
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()
	listDayOffs := usecase.ListDayOffs{Store: dos, Settings: settings, Loc: time.UTC}
	statsUC := usecase.StatsComputer{
		Sessions: sessions,
		Settings: settings,
		DayOffs:  listDayOffs,
		Clock:    clk,
		Loc:      time.UTC,
	}
	srv := &httpserver.Server{
		Ensure:      usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:         bus,
		Clock:       clk,
		Users:       users,
		Session:     codec,
		Stats:       statsUC,
		SetTarget:   usecase.SetTargetConfig{Settings: settings},
		GetSettings: usecase.GetSettings{Settings: settings, Tokens: tokens},
	}
	return srv, codec, settings, sessions
}

func TestWebStatsHome(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/stats", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "flow · stats") {
		t.Fatalf("expected 'flow · stats' in body, got: %.200s", body)
	}
	if !strings.Contains(body, "Woche") {
		t.Fatalf("expected 'Woche' in body, got: %.200s", body)
	}
}

func TestWebStatsFragment(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/ui/stats/fragment", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/stats/fragment status=%d body=%.200s", res.StatusCode, body)
	}
	// Monat tile is always present in the stats fragment.
	if !strings.Contains(body, "Monat") {
		t.Fatalf("expected 'Monat' in body, got: %.200s", body)
	}
}

func TestWebStatsHome_WithOvertime(t *testing.T) {
	// Pre-seed a session with >8h (default target) so clampPct(>100) and
	// fmtSaldo positive branches are exercised.
	srv, codec, settings, sessions := newWebStatsServerFull(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	// Set a low target (60 min) so a 2-hour session = 200% → clampPct(200)→100.
	_ = settings.SetTargetConfig(context.Background(), "u1", 60, map[time.Weekday]int{})

	// Seed a 2h stopped session today (2026-06-15, Monday).
	start := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{
		ID: "over-1", OwnerID: "u1", Start: start, Stop: &stop,
	})

	req, _ := http.NewRequest("GET", ts.URL+"/stats", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats (overtime) status=%d body=%.200s", res.StatusCode, body)
	}
	// Positive saldo rendered as "+HH:MM".
	if !strings.Contains(body, "+") {
		t.Fatalf("expected positive saldo (+) in overtime body, got: %.300s", body)
	}
}

func TestWebStatsHome_RendersMonatTile(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/stats", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats status=%d body=%.200s", res.StatusCode, body)
	}
	// Monat tile is always present in the stats page.
	if !strings.Contains(body, "Monat") {
		t.Fatalf("expected 'Monat' in body, got: %.200s", body)
	}
}

func TestWebStatsFragment_WeekRange(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/ui/stats/fragment?range=week", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/stats/fragment?range=week status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "Woche") {
		t.Fatalf("expected 'Woche' label for week range, got: %.200s", body)
	}
}

func TestWebSetTarget_EmptyWeekdayClearsOverride(t *testing.T) {
	srv, codec, settingsStore := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	// Seed a Friday override, then POST WITHOUT any weekday field → the form is
	// authoritative, so the override is cleared (empty = inherit default).
	ctx := context.Background()
	_ = settingsStore.SetTargetConfig(ctx, "u1", 480, map[time.Weekday]int{time.Friday: 240})

	form := url.Values{"defaultTargetMin": {"420"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	stored, _ := settingsStore.Get(ctx, "u1")
	if stored.DefaultTargetMin != 420 {
		t.Errorf("want default 420, got %d", stored.DefaultTargetMin)
	}
	if _, ok := stored.WeekdayTargetMin[time.Friday]; ok {
		t.Errorf("Friday override should be cleared, got map=%v", stored.WeekdayTargetMin)
	}
}

func TestWebSetTarget_InvalidWeekday(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"defaultTargetMin": {"480"}, "mon": {"-5"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for negative weekday, got %d", res.StatusCode)
	}
}

func TestWebSetTarget_InvalidDefaultMin(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"defaultTargetMin": {"not-a-number"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid defaultTargetMin, got %d", res.StatusCode)
	}
}

func TestWebSetTarget(t *testing.T) {
	srv, codec, settingsStore := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	ctx := context.Background()

	// POST default + a Friday override → both persist exactly as posted.
	form := url.Values{
		"defaultTargetMin": {"360"},
		"fri":              {"300"},
	}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /ui/stats/target status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "Heute") { // fragment marker (saldo tile label)
		t.Fatalf("expected 'Heute' (fragment marker) in body, got: %.200s", body)
	}
	stored, err := settingsStore.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("reading stored settings: %v", err)
	}
	if stored.DefaultTargetMin != 360 {
		t.Errorf("want DefaultTargetMin=360, got %d", stored.DefaultTargetMin)
	}
	if v, ok := stored.WeekdayTargetMin[time.Friday]; !ok || v != 300 {
		t.Errorf("Friday override should be 300, got map=%v", stored.WeekdayTargetMin)
	}
}
