package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWorktimeStatusServer wires a server with the WorktimeStatus composer over
// shared fakes; returns the session + day-off stores so tests can pre-seed. The
// authenticated user (sub-1) resolves to id "id-1" (FakeIDGen, first NewID).
func newWorktimeStatusServer(subject string) (*httpserver.Server, *testutil.FakeSessionStore, *testutil.FakeDayOffStore) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)} // Monday
	bus := sse.NewBus()
	settings := testutil.NewFakeUserSettingsStore()
	sessions := testutil.NewFakeSessionStore()
	dayOffs := testutil.NewFakeDayOffStore()
	nodes := testutil.NewFakeNodeStore()
	ids := &testutil.FakeIDGen{}
	listDayOffs := usecase.ListDayOffs{Store: dayOffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC, Nodes: nodes}
	srv := &httpserver.Server{
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: subject, Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               bus,
		Emitter:           sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:             clk,
		Stats:             stats,
		GetRunningSession: usecase.GetRunningSession{Sessions: sessions},
		ListDayOffs:       listDayOffs,
		WorktimeStatus:    usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC},
	}
	return srv, sessions, dayOffs
}

func TestHandleWorktimeStatus_ShapeAndAuth(t *testing.T) {
	srv, _, _ := newWorktimeStatusServer("sub-1")
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// unauth → 401
	res, _ := http.Get(ts.URL + "/api/v1/worktime/status")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth want 401, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	for _, k := range []string{"date", "loggedMin", "targetMin", "running", "week", "streak", "burndown"} {
		if _, ok := got[k]; !ok {
			t.Errorf("DTO missing key %q", k)
		}
	}
	// empty day → week is a non-empty array (7 rows), running false.
	if wk, ok := got["week"].([]any); !ok || len(wk) != 7 {
		t.Errorf("week should be a 7-element array, got %v", got["week"])
	}
	if got["running"] != false {
		t.Errorf("empty day should be running=false, got %v", got["running"])
	}
}

func TestHandleWorktimeStatus_RunningFields(t *testing.T) {
	srv, sessions, _ := newWorktimeStatusServer("sub-1")
	start := time.Date(2026, 6, 15, 9, 30, 0, 0, time.UTC)
	if _, err := sessions.Create(context.Background(), domain.WorkSession{ID: "run-1", OwnerID: "id-1", Start: start}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, _ := http.DefaultClient.Do(req)
	defer func() { _ = res.Body.Close() }()
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got["running"] != true {
		t.Fatalf("want running=true, got %v", got["running"])
	}
	if got["activeSessionId"] != "run-1" {
		t.Errorf("activeSessionId = %v, want run-1", got["activeSessionId"])
	}
	if s, _ := got["activeStart"].(string); s == "" {
		t.Errorf("activeStart must be set when running, got %q", s)
	}
}

func TestHandleWorktimeStatus_DayOffShape(t *testing.T) {
	srv, _, dayOffs := newWorktimeStatusServer("sub-1")
	today := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := dayOffs.Add(context.Background(), "id-1", domain.DayOff{Date: today, Kind: domain.KindVacation, Label: "Urlaub"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, _ := http.DefaultClient.Do(req)
	defer func() { _ = res.Body.Close() }()
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	dayOff, ok := got["dayOff"].(map[string]any)
	if !ok {
		t.Fatalf("dayOff must be an object today, got %v", got["dayOff"])
	}
	if dayOff["kind"] != "vacation" || dayOff["label"] != "Urlaub" {
		t.Errorf("dayOff shape wrong: %v", dayOff)
	}
}

// errListSessions wraps a working fake but fails List — drives the handler's
// 500 path through the composer.
type errListSessions struct {
	*testutil.FakeSessionStore
}

func (e errListSessions) List(context.Context, string, time.Time) ([]domain.WorkSession, error) {
	return nil, errors.New("boom")
}

func TestHandleWorktimeStatus_ReaderErrorIs500(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	settings := testutil.NewFakeUserSettingsStore()
	sessions := errListSessions{testutil.NewFakeSessionStore()}
	dayOffs := testutil.NewFakeDayOffStore()
	ids := &testutil.FakeIDGen{}
	listDayOffs := usecase.ListDayOffs{Store: dayOffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC}
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(), Clock: clk,
		WorktimeStatus: usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, _ := http.DefaultClient.Do(req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("reader error must be 500, got %d", res.StatusCode)
	}
}

// Handler owner-scoping (AGENTS.md): a running session belonging to a FOREIGN
// owner (id-2) must never surface for the authenticated user (id-1).
func TestHandleWorktimeStatus_HandlerOwnerScoped(t *testing.T) {
	srv, sessions, _ := newWorktimeStatusServer("sub-1")
	// Foreign owner id-2 has a running session; authed user id-1 has none.
	if _, err := sessions.Create(context.Background(), domain.WorkSession{ID: "foreign", OwnerID: "id-2", Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, _ := http.DefaultClient.Do(req)
	defer func() { _ = res.Body.Close() }()
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got["running"] != false {
		t.Fatalf("authed user must not see foreign running session: running=%v", got["running"])
	}
}
