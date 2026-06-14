package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newFeedServer() *httpserver.Server {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	tokens := testutil.NewFakeFeedTokenStore()
	dos := testutil.NewFakeDayOffStore()
	return &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:        usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		RegenIcsToken: usecase.RegenerateIcsToken{Tokens: tokens, Clock: clk},
		IcsFeed:       usecase.IcsFeed{Tokens: tokens, Store: dos, Clock: clk},
	}
}

func TestIcsFeed_UnknownToken404(t *testing.T) {
	ts := httptest.NewServer(newFeedServer().Routes())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/ics/does-not-exist.ics")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token: want 404, got %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestIcsFeed_ValidTokenServesCalendar(t *testing.T) {
	ts := httptest.NewServer(newFeedServer().Routes())
	defer ts.Close()

	// Mint a token via the authenticated regenerate endpoint.
	rreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/ics-token/regenerate", nil)
	rreq.Header.Set("Authorization", "Bearer x")
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil || rres.StatusCode != http.StatusOK {
		t.Fatalf("regen status=%v err=%v", rres.StatusCode, err)
	}
	var body struct {
		Token   string `json:"token"`
		FeedURL string `json:"feedUrl"`
	}
	_ = json.NewDecoder(rres.Body).Decode(&body)
	_ = rres.Body.Close()
	if body.Token == "" {
		t.Fatal("empty token from regenerate")
	}

	// Fetch the feed with NO auth header (token-by-URL).
	res, err := http.Get(ts.URL + "/ics/" + body.Token + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("valid token: want 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("content-type = %q, want text/calendar", ct)
	}
}
