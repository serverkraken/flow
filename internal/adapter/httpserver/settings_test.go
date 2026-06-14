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

func newSettingsServer() *httpserver.Server {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()
	return &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:        usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		GetSettings:   usecase.GetSettings{Settings: settings, Tokens: tokens},
		SetBundesland: usecase.SetBundesland{Settings: settings},
		RegenIcsToken: usecase.RegenerateIcsToken{Tokens: tokens, Clock: clk},
	}
}

func TestSettingsAndIcsToken(t *testing.T) {
	ts := httptest.NewServer(newSettingsServer().Routes())
	defer ts.Close()

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Set Bundesland.
	res := do("POST", "/api/v1/settings/bundesland", `{"bundesland":"BY"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("set bundesland status %d, want 204", res.StatusCode)
	}
	_ = res.Body.Close()

	// Reject garbage.
	res = do("POST", "/api/v1/settings/bundesland", `{"bundesland":"XX"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage bundesland status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// Regenerate token returns a non-empty secret + feed URL.
	res = do("POST", "/api/v1/ics-token/regenerate", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("regen status %d, want 200", res.StatusCode)
	}
	var tok struct {
		Token   string `json:"token"`
		FeedURL string `json:"feedUrl"`
	}
	_ = json.NewDecoder(res.Body).Decode(&tok)
	_ = res.Body.Close()
	if tok.Token == "" || !strings.Contains(tok.FeedURL, "/ics/") || !strings.HasSuffix(tok.FeedURL, ".ics") {
		t.Fatalf("bad token response: %+v", tok)
	}

	// GET settings reflects BY + the new feed URL.
	res = do("GET", "/api/v1/settings", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get settings status %d", res.StatusCode)
	}
	var set struct {
		Bundesland string   `json:"bundesland"`
		FeedURLs   []string `json:"feedUrls"`
	}
	_ = json.NewDecoder(res.Body).Decode(&set)
	_ = res.Body.Close()
	if set.Bundesland != "BY" || len(set.FeedURLs) != 1 {
		t.Fatalf("settings = %+v, want BY + 1 feed url", set)
	}
}
