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
func newWebStatsServer(t *testing.T) (*httpserver.Server, *websession.Codec) {
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
	return srv, codec
}

func TestWebStatsHome(t *testing.T) {
	srv, codec := newWebStatsServer(t)
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
	srv, codec := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/ui/stats/fragment?range=month", nil)
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
	if !strings.Contains(body, "Monat") {
		t.Fatalf("expected 'Monat' in body, got: %.200s", body)
	}
}

func TestWebSetTarget(t *testing.T) {
	srv, codec := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"defaultTargetMin": {"360"}}.Encode()
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
	// Response should be the fragment re-render, containing "Heute" from StatsFragment.
	if !strings.Contains(body, "Heute") {
		t.Fatalf("expected 'Heute' (fragment marker) in body, got: %.200s", body)
	}
}
