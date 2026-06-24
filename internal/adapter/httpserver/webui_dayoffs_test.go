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

// newWebDayOffServer wires the dayoff web handlers behind cookie auth, with a
// pre-seeded user "u1" whose session cookie the test forges via the codec.
func newWebDayOffServer(t *testing.T) (*httpserver.Server, *websession.Codec) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	dos := testutil.NewFakeDayOffStore()
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()
	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:           bus,
		Clock:         clk,
		Users:         users,
		Session:       codec,
		AddDayOffs:    usecase.AddDayOffs{Store: dos, Bus: bus},
		DeleteDayOff:  usecase.DeleteDayOff{Store: dos, Bus: bus},
		ListDayOffs:   usecase.ListDayOffs{Store: dos, Settings: settings, Loc: time.UTC},
		GetSettings:   usecase.GetSettings{Settings: settings, Tokens: tokens},
		SetBundesland: usecase.SetBundesland{Settings: settings},
		IcsFeed:       usecase.IcsFeed{Tokens: tokens, Store: dos, Clock: clk},
		RegenIcsToken: usecase.RegenerateIcsToken{Tokens: tokens, Clock: clk},
	}
	return srv, codec
}

func TestWebDayOffPageAndMutations(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		return res.StatusCode, string(b)
	}

	// Full page renders (covers FreiPage templ + dayOffData).
	code, body := do("GET", "/dayoffs", "")
	if code != http.StatusOK || !strings.Contains(body, "flow · frei") {
		t.Fatalf("GET /dayoffs status=%d body=%.120s", code, body)
	}

	// Add a vacation week → fragment shows the entries.
	form := url.Values{
		"from": {"2026-06-15"}, "to": {"2026-06-19"},
		"kind": {"vacation"}, "label": {"Sommer"}, "skipWeekends": {"true"},
	}.Encode()
	code, body = do("POST", "/ui/dayoffs/add", form)
	if code != http.StatusOK || !strings.Contains(body, "15.06.2026") {
		t.Fatalf("add status=%d body=%.200s", code, body)
	}

	// Regenerate the ICS token → fragment shows a feed URL.
	code, body = do("POST", "/ui/dayoffs/regen-token", "")
	if code != http.StatusOK || !strings.Contains(body, "/ics/") {
		t.Fatalf("regen status=%d body=%.200s", code, body)
	}

	// Fragment endpoint renders standalone.
	code, body = do("GET", "/ui/dayoffs", "")
	if code != http.StatusOK || !strings.Contains(body, "name=\"bundesland\"") {
		t.Fatalf("fragment status=%d body=%.120s", code, body)
	}

	// Delete the day → still renders.
	code, _ = do("POST", "/ui/dayoffs/delete", url.Values{"day": {"2026-06-15"}}.Encode())
	if code != http.StatusOK {
		t.Fatalf("delete status=%d", code)
	}
}

func TestWebDayOffPage_ListsAllManualKinds(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/dayoffs", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	for _, want := range []string{"Urlaub", "Krank", "Gleittag", "Sonderurlaub", "Kind krank", "Fortbildung"} {
		if !strings.Contains(body, want) {
			t.Fatalf("day-offs page select missing %q;\n%.400s", want, body)
		}
	}
}
