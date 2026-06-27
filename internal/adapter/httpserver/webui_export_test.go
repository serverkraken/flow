package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

// newWebExportServer wires the export web handlers behind cookie auth, with a
// pre-seeded user "u1" whose session cookie the test forges via the codec.
// Returns the server, codec, session store and project store for seeding.
func newWebExportServer(t *testing.T) (*httpserver.Server, *websession.Codec, *testutil.FakeSessionStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	sessions := testutil.NewFakeSessionStore()
	projects := testutil.NewFakeNodeStore()
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()

	srv := &httpserver.Server{
		Ensure:      usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:         bus,
		Clock:       clk,
		Users:       users,
		Session:     codec,
		GetSettings: usecase.GetSettings{Settings: settings, Tokens: tokens},
		BuildExport: usecase.BuildExport{
			Sessions: sessions,
			Nodes: projects,
			Clock:    clk,
			Loc:      time.UTC,
		},
	}
	return srv, codec, sessions, projects
}

func TestWebExportHome(t *testing.T) {
	srv, codec, _, _ := newWebExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/export", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /export status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "flow · export") {
		t.Fatalf("expected 'flow · export' in body, got: %.200s", body)
	}
	if !strings.Contains(body, "export") {
		t.Fatalf("expected 'export' nav link in body, got: %.200s", body)
	}
}

func TestWebExportPreview(t *testing.T) {
	srv, codec, sessions, projects := newWebExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	// Seed one project and one stopped session so the summary has a row.
	projID := "proj-webexport-1"
	proj := domain.Node{
		ID: projID, OwnerID: "u1", Name: "WebTestProject",
		Slug: "webtestproject", Status: domain.NodeActive,
	}
	if _, err := projects.Create(context.Background(), proj); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	ws := domain.WorkSession{
		ID: "sess-webexport-1", OwnerID: "u1",
		Start: start, Stop: &stop, NodeID: &projID,
	}
	if _, err := sessions.Create(context.Background(), ws); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	req, _ := http.NewRequest("GET", ts.URL+"/ui/export/preview?from=2026-06-01&to=2026-06-30", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/export/preview status=%d body=%.200s", res.StatusCode, body)
	}
	if strings.Contains(body, "<!DOCTYPE") {
		t.Error("preview should return a fragment, not a full page")
	}
	if !strings.Contains(body, "WebTestProject") {
		t.Fatalf("expected seeded project 'WebTestProject' in body, got: %.200s", body)
	}
	// The seeded session runs 9:00–11:00 = 2h 00m.
	if !strings.Contains(body, "2h 00m") {
		t.Errorf("expected seeded duration '2h 00m' in fragment body, got: %.400s", body)
	}
}
