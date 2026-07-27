package httpserver_test

import (
	"context"
	"errors"
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

type failingWebDayOffStore struct {
	*testutil.FakeDayOffStore
	addErr    error
	deleteErr error
}

func (s failingWebDayOffStore) Add(ctx context.Context, ownerID string, d domain.DayOff) error {
	if s.addErr != nil {
		return s.addErr
	}
	return s.FakeDayOffStore.Add(ctx, ownerID, d)
}

func (s failingWebDayOffStore) Delete(ctx context.Context, ownerID string, day time.Time) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.FakeDayOffStore.Delete(ctx, ownerID, day)
}

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
	ids := &testutil.FakeIDGen{}
	emitter := sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk)
	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           bus,
		Emitter:       emitter,
		Clock:         clk,
		Users:         users,
		Session:       codec,
		AddDayOffs:    usecase.AddDayOffs{Store: dos, Emitter: emitter},
		DeleteDayOff:  usecase.DeleteDayOff{Store: dos, Emitter: emitter},
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
	// L4 Task 6: Frei is now a full Lesesaal page (pagehead/panel/row), not
	// the retired Kristall glass cards.
	content := nonDialogContent(body)
	for _, want := range []string{"pagehead", "panel", "spine", "‹ Zeit"} {
		if !strings.Contains(content, want) {
			t.Errorf("Frei page-flow content missing %q, got:\n%.800s", want, content)
		}
	}
	// "font-display" also appears in the shared AppShell topbar logo mark
	// (unrelated to Frei content, see Woche/Historie precedent), so it's
	// excluded here and checked below via the Frei-owned fragment endpoint.
	for _, unwanted := range []string{"glass", "shadow-soft", "rounded-3xl"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("Frei page-flow content must not render retired Kristall chrome %q, got:\n%.800s", unwanted, content)
		}
	}
	// Add-form, Bundesland select, and ICS copy button must still render.
	if !strings.Contains(body, `hx-post="/ui/dayoffs/add"`) {
		t.Errorf("add-form should still post to /ui/dayoffs/add, got:\n%.400s", body)
	}
	if !strings.Contains(body, `name="bundesland"`) {
		t.Errorf("Bundesland select should still render, got:\n%.400s", body)
	}
	if !strings.Contains(body, "data-copy=") {
		t.Errorf("ICS copy button should still render, got:\n%.400s", body)
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
	// The added entry renders as a Lesesaal .row with a .typechip kind tone,
	// not the retired freiKindChip bg-{hue}/10 wash.
	if !strings.Contains(body, `class="row"`) || !strings.Contains(body, "typechip") {
		t.Errorf("expected the day-off list to render .row/.typechip entries, got:\n%.600s", body)
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
	// The Frei-owned fragment (no shared topbar chrome) must not render any
	// of the retired Kristall classes, including font-display, in its
	// page-flow content — scoped before the first native <dialog> (the
	// ConfirmDialogs legitimately keep their own modal chrome).
	fragContent := nonDialogContent(body)
	for _, unwanted := range []string{"glass", "shadow-soft", "rounded-3xl", "font-display"} {
		if strings.Contains(fragContent, unwanted) {
			t.Errorf("Frei fragment must not render retired Kristall chrome %q, got:\n%.800s", unwanted, fragContent)
		}
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

func TestWebSetBundesland(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	do := func(body string) (int, string) {
		req, _ := http.NewRequest("POST", ts.URL+"/ui/dayoffs/bundesland", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		return res.StatusCode, string(b)
	}

	// Switch to NW → 200, fragment header names the state + NW holidays appear.
	code, body := do(url.Values{"bundesland": {"NW"}}.Encode())
	if code != http.StatusOK {
		t.Fatalf("set bundesland status=%d body=%.200s", code, body)
	}
	if !strings.Contains(body, "Nordrhein-Westfalen") {
		t.Fatalf("fragment should name the new state, got: %.300s", body)
	}
	if !strings.Contains(body, "Fronleichnam") { // NW-specific holiday → recomputed
		t.Fatalf("NW holidays should recompute (expected Fronleichnam), got: %.300s", body)
	}

	// Invalid code → 400.
	code, _ = do(url.Values{"bundesland": {"XX"}}.Encode())
	if code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid bundesland, got %d", code)
	}
}

func TestWebFormsRejectMalformedAndOversizedBodiesBeforeMutation(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "malformed", body: "bundesland=NW&bad=%zz", want: http.StatusBadRequest},
		{name: "oversized", body: "bundesland=NW&padding=" + strings.Repeat("x", 2*1024*1024), want: http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ui/dayoffs/bundesland", strings.NewReader(tc.body))
			req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestWebDayOffMutationErrorsRenderVisibleAlert(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		form  url.Values
		store failingWebDayOffStore
	}{
		{
			name: "add store failure", path: "/ui/dayoffs/add",
			form:  url.Values{"from": {"2026-06-15"}, "to": {"2026-06-15"}, "kind": {"vacation"}},
			store: failingWebDayOffStore{FakeDayOffStore: testutil.NewFakeDayOffStore(), addErr: errors.New("write failed")},
		},
		{
			name: "delete store failure", path: "/ui/dayoffs/delete",
			form:  url.Values{"day": {"2026-06-15"}},
			store: failingWebDayOffStore{FakeDayOffStore: testutil.NewFakeDayOffStore(), deleteErr: errors.New("delete failed")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, codec := newWebDayOffServer(t)
			srv.AddDayOffs.Store = tc.store
			srv.DeleteDayOff.Store = tc.store
			ts := httptest.NewServer(srv.Routes())
			defer ts.Close()
			cookieVal, _ := codec.Issue("u1")

			req, _ := http.NewRequest(http.MethodPost, ts.URL+tc.path, strings.NewReader(tc.form.Encode()))
			req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `role="alert"`) {
				t.Fatalf("status=%d body=%.400s, want visible alert fragment", res.StatusCode, body)
			}
		})
	}
}
