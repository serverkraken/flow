package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newExportServer builds a Server wired with BuildExport + SetNodeRate using
// in-memory fakes. Returns the server, the session store and project store so
// tests can pre-seed data.
func newExportServer(t *testing.T) (*httpserver.Server, *testutil.FakeSessionStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	bus := sse.NewBus()
	sessions := testutil.NewFakeSessionStore()
	projects := testutil.NewFakeNodeStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayOffs := testutil.NewFakeDayOffStore()
	listDayOffs := usecase.ListDayOffs{Store: dayOffs, Settings: settings, Loc: time.UTC}
	statsUC := usecase.StatsComputer{
		Sessions: sessions,
		Settings: settings,
		DayOffs:  listDayOffs,
		Clock:    clk,
		Loc:      time.UTC,
	}

	srv := &httpserver.Server{
		Verifier:  testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:    usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:       bus,
		Clock:     clk,
		Stats:     statsUC,
		SetTarget: usecase.SetTargetConfig{Settings: settings},
		BuildExport: usecase.BuildExport{
			Sessions: sessions,
			Nodes: projects,
			Clock:    clk,
			Loc:      time.UTC,
		},
		SetNodeRate: usecase.SetNodeRate{Nodes: projects},
	}
	return srv, sessions, projects
}

// seedExportData pre-seeds one project and one booked (stopped) session under
// ownerID so export endpoints return non-empty results. Returns the project ID.
func seedExportData(t *testing.T, sessions *testutil.FakeSessionStore, projects *testutil.FakeNodeStore, ownerID string) string {
	t.Helper()
	projID := "proj-export-1"
	proj := domain.Node{
		ID:      projID,
		OwnerID: ownerID,
		Name:    "TestProject",
		Slug:    "testproject",
		Status:  domain.NodeActive,
	}
	if _, err := projects.Create(context.Background(), proj); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	ws := domain.WorkSession{
		ID:        "sess-export-1",
		OwnerID:   ownerID,
		Start:     start,
		Stop:      &stop,
		NodeID: &projID,
	}
	if _, err := sessions.Create(context.Background(), ws); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return projID
}

func TestHandleExport_CSV(t *testing.T) {
	srv, sessions, projects := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Prime user creation (EnsureUser → "id-1").
	primeUser(t, ts.URL)
	seedExportData(t, sessions, projects, "id-1")

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export?from=2026-06-01&to=2026-06-30&format=csv", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type: want text/csv, got %q", ct)
	}
	cd := res.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition: want attachment, got %q", cd)
	}
	tmp := make([]byte, 512)
	n, _ := res.Body.Read(tmp)
	body := string(tmp[:n])
	if !strings.HasPrefix(body, "date,start,stop,duration_seconds,project,tag,note") {
		t.Errorf("CSV body should start with header, got: %q", body)
	}
}

func TestHandleExport_JSON(t *testing.T) {
	srv, sessions, projects := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)
	seedExportData(t, sessions, projects, "id-1")

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export?from=2026-06-01&to=2026-06-30&format=json", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
	body := make([]byte, 4096)
	n, _ := res.Body.Read(body)
	bodyStr := string(body[:n])
	if !strings.Contains(bodyStr, `"byProject"`) {
		t.Errorf("JSON body should contain byProject, got: %q", bodyStr)
	}
}

func TestHandleExport_MD(t *testing.T) {
	srv, sessions, projects := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)
	seedExportData(t, sessions, projects, "id-1")

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export?from=2026-06-01&to=2026-06-30&format=md", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/markdown") {
		t.Errorf("Content-Type: want text/markdown, got %q", ct)
	}
	body := make([]byte, 4096)
	n, _ := res.Body.Read(body)
	bodyStr := string(body[:n])
	if !strings.Contains(bodyStr, "# Worktime") {
		t.Errorf("MD body should contain '# Worktime', got: %q", bodyStr)
	}
}

func TestHandleExport_BadFormat(t *testing.T) {
	srv, _, _ := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export?from=2026-06-01&to=2026-06-30&format=xml", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid format, got %d", res.StatusCode)
	}
}

func TestHandleExport_NoRange(t *testing.T) {
	srv, _, _ := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing range, got %d", res.StatusCode)
	}
}

func TestHandleExport_ToBeforeFrom(t *testing.T) {
	srv, _, _ := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/export?from=2026-06-30&to=2026-06-01&format=csv", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for to<from, got %d", res.StatusCode)
	}
}

func TestHandleSetProjectRate(t *testing.T) {
	srv, sessions, projects := newExportServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Prime user so "id-1" is created, then seed a project under it.
	primeUser(t, ts.URL)
	projID := seedExportData(t, sessions, projects, "id-1")

	// Happy path: set a valid rate.
	body := `{"amount":8000,"currency":"EUR"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/nodes/"+projID+"/rate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", res.StatusCode)
	}

	// Invalid rate: negative amount.
	body2 := `{"amount":-1,"currency":"EUR"}`
	req2, _ := http.NewRequest("POST", ts.URL+"/api/v1/nodes/"+projID+"/rate", strings.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer x")
	req2.Header.Set("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for negative amount, got %d", res2.StatusCode)
	}

	// Unknown project: expect 404.
	body3 := `{"amount":5000,"currency":"EUR"}`
	req3, _ := http.NewRequest("POST", ts.URL+"/api/v1/nodes/no-such-project/rate", strings.NewReader(body3))
	req3.Header.Set("Authorization", "Bearer x")
	req3.Header.Set("Content-Type", "application/json")
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	_ = res3.Body.Close()
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for unknown project, got %d", res3.StatusCode)
	}

	// Clear rate: amount=null clears the stored rate (→ 204, rate becomes nil).
	body4 := `{"amount":null,"currency":""}`
	req4, _ := http.NewRequest("POST", ts.URL+"/api/v1/nodes/"+projID+"/rate", strings.NewReader(body4))
	req4.Header.Set("Authorization", "Bearer x")
	req4.Header.Set("Content-Type", "application/json")
	res4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	_ = res4.Body.Close()
	if res4.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204 for clear rate, got %d", res4.StatusCode)
	}
	got, err := projects.Get(context.Background(), "id-1", projID)
	if err != nil {
		t.Fatalf("get project after clear: %v", err)
	}
	if got.Rate != nil {
		t.Errorf("want Rate nil after clear, got %+v", got.Rate)
	}
}
