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

// newWebEinstellungenServer wires the Einstellungen web handlers behind cookie
// auth, with a pre-seeded user "u1" whose session cookie the test forges via
// the codec. It also returns the FakeUserSettingsStore so tests can inspect
// stored state.
func newWebEinstellungenServer(t *testing.T) (*httpserver.Server, *websession.Codec, *testutil.FakeUserSettingsStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()
	ids := &testutil.FakeIDGen{}
	srv := &httpserver.Server{
		Ensure:      usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:         bus,
		Emitter:     sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:       clk,
		Users:       users,
		Session:     codec,
		SetTarget:   usecase.SetTargetConfig{Settings: settings},
		GetSettings: usecase.GetSettings{Settings: settings, Tokens: tokens},
	}
	return srv, codec, settings
}

func TestWebEinstellungenHome(t *testing.T) {
	srv, codec, _ := newWebEinstellungenServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/einstellungen", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /einstellungen status=%d body=%.300s", res.StatusCode, body)
	}
	// Page renders the default-target input.
	if !strings.Contains(body, "defaultTargetMin") {
		t.Fatalf("expected 'defaultTargetMin' input in body, got: %.300s", body)
	}
	// Page heading shows "Einstellungen".
	if !strings.Contains(body, "Einstellungen") {
		t.Fatalf("expected 'Einstellungen' in body, got: %.300s", body)
	}
}

func TestWebSetTargetEinst(t *testing.T) {
	srv, codec, settingsStore := newWebEinstellungenServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	ctx := context.Background()

	form := url.Values{
		"defaultTargetMin": {"360"},
		"fri":              {"300"},
	}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/einstellungen/target", strings.NewReader(form))
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
		t.Fatalf("POST /ui/einstellungen/target status=%d body=%.300s", res.StatusCode, body)
	}
	// Response is the Einstellungen target fragment (contains the form input).
	if !strings.Contains(body, "defaultTargetMin") {
		t.Fatalf("expected 'defaultTargetMin' (fragment) in body, got: %.300s", body)
	}
	// Verify settings were persisted.
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

func TestWebSetTargetEinst_InvalidDefault(t *testing.T) {
	srv, codec, _ := newWebEinstellungenServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"defaultTargetMin": {"not-a-number"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/einstellungen/target", strings.NewReader(form))
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

func TestWebEinstellungenInputsUseFieldClass(t *testing.T) {
	srv, codec, _ := newWebEinstellungenServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/einstellungen", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	// Check that the target form fragment contains the field class
	if !strings.Contains(body, "class=\"field") {
		t.Fatalf("expected 'field' class in Einstellungen form, got: %.500s", body)
	}
	// Check that the old inline styling (bg-sunken/60) is NOT used in the form
	if strings.Contains(body, "bg-sunken/60") {
		t.Fatalf("found legacy 'bg-sunken/60' style in form (should use .field class instead), got: %.500s", body)
	}
}
