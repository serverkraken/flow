package httpserver_test

import (
	"context"
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

// worktimeTestServer holds the test server + helpers for mutating worktime
// WebUI handlers (add/edit/delete).
type worktimeTestServer struct {
	ts    *httptest.Server
	srv   *httpserver.Server
	ss    *testutil.FakeSessionStore
	ps    *testutil.FakeProjectStore
	ids   *testutil.FakeIDGen
	clk   testutil.FakeClock
	codec *websession.Codec
}

func newWorktimeTestServer(t *testing.T) *worktimeTestServer {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	dos := testutil.NewFakeDayOffStore()
	settings := testutil.NewFakeUserSettingsStore()
	tokens := testutil.NewFakeFeedTokenStore()
	listDayOffs := usecase.ListDayOffs{Store: dos, Settings: settings, Loc: time.Local}
	srv := &httpserver.Server{
		Ensure:              usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:                 bus,
		Clock:               clk,
		Users:               users,
		Session:             codec,
		StartSession:        usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:         usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:        usecase.ListSessions{Sessions: ss, Clock: clk},
		ListSessionsRange:   usecase.ListSessionsRange{Sessions: ss},
		ListSessionsPage:    usecase.ListSessionsPage{Sessions: ss},
		BulkAssignProject:   usecase.BulkAssignProject{Sessions: ss, Projects: ps},
		BulkDeleteSessions:  usecase.BulkDeleteSessions{Sessions: ss},
		AddSession:          usecase.AddSession{Sessions: ss, IDs: ids, Clock: clk},
		EditSession:         usecase.EditSession{Sessions: ss},
		DeleteSession:       usecase.DeleteSession{Sessions: ss},
		CreateProject:       usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:        usecase.ListProjects{Projects: ps},
		ListProjectBindings: usecase.ListProjectBindings{Bindings: bs},
		ListDayOffs:         listDayOffs,
		GetSettings:         usecase.GetSettings{Settings: settings, Tokens: tokens},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Settings: settings,
			DayOffs:  listDayOffs,
			Clock:    clk,
			Loc:      time.Local,
		},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return &worktimeTestServer{ts: ts, srv: srv, ss: ss, ps: ps, ids: ids, clk: clk, codec: codec}
}

// postForm posts a form to the given path as authenticated user "u1" and
// returns the response recorder (uses httptest.ResponseRecorder, not real HTTP).
func (w *worktimeTestServer) postForm(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	cookieVal, _ := w.codec.Issue("u1")
	req, _ := http.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	w.srv.Routes().ServeHTTP(rec, req)
	return rec
}

// seedSession creates a completed session on the given date via AddSession.
func (w *worktimeTestServer) seedSession(t *testing.T, dateStr, fromHHMM, toHHMM string) {
	t.Helper()
	day, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		t.Fatalf("seedSession: parse date %q: %v", dateStr, err)
	}
	parseHM := func(hhmm string) time.Time {
		clk, _ := time.ParseInLocation("15:04", hhmm, time.Local)
		return time.Date(day.Year(), day.Month(), day.Day(), clk.Hour(), clk.Minute(), 0, 0, time.Local)
	}
	_, err = usecase.AddSession{Sessions: w.ss, IDs: w.ids, Clock: w.clk}.Execute(
		context.Background(), "u1", nil, parseHM(fromHHMM), parseHM(toHHMM), "", "",
	)
	if err != nil {
		t.Fatalf("seedSession: %v", err)
	}
}

func TestWebAdd_BackfillsSession(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// The Heute fragment is today-scoped (clock = 2026-06-21), so backfill today.
	form := url.Values{
		"date": {"2026-06-21"}, "from": {"09:00"}, "to": {"11:00"},
	}
	res := srv.postForm(t, "/ui/worktime/add", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "09:00–11:00") {
		t.Errorf("expected new session row, got:\n%s", res.Body.String())
	}
}

func TestWebAdd_OverlapShowsError(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-18", "09:00", "12:00")
	form := url.Values{"date": {"2026-06-18"}, "from": {"10:00"}, "to": {"11:00"}}
	res := srv.postForm(t, "/ui/worktime/add", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d (want 200 with inline error)", res.Code)
	}
	if !strings.Contains(strings.ToLower(res.Body.String()), "overlap") {
		t.Errorf("expected overlap error banner, got:\n%s", res.Body.String())
	}
}

func TestWebDelete_RemovesSession(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-18", "09:00", "11:00")
	// Find the session id from the store by listing.
	sessions, _ := usecase.ListSessionsRange{Sessions: srv.ss}.Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 19, 0, 0, 0, 0, time.Local),
	)
	if len(sessions) != 1 {
		t.Fatalf("seed: expected 1 session, got %d", len(sessions))
	}
	sid := sessions[0].ID
	form := url.Values{"date": {"2026-06-18"}, "sessionId": {sid}}
	res := srv.postForm(t, "/ui/worktime/delete", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	// Session should be gone.
	after, _ := usecase.ListSessionsRange{Sessions: srv.ss}.Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 19, 0, 0, 0, 0, time.Local),
	)
	if len(after) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(after))
	}
}

func TestWebEdit_UpdatesStop(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// Today-scoped Heute fragment (clock = 2026-06-21): seed + edit today.
	srv.seedSession(t, "2026-06-21", "09:00", "11:00")
	sessions, _ := usecase.ListSessionsRange{Sessions: srv.ss}.Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 22, 0, 0, 0, 0, time.Local),
	)
	if len(sessions) != 1 {
		t.Fatalf("seed: expected 1 session, got %d", len(sessions))
	}
	sid := sessions[0].ID
	form := url.Values{
		"date": {"2026-06-21"}, "sessionId": {sid},
		"from": {"09:00"}, "to": {"12:30"},
	}
	res := srv.postForm(t, "/ui/worktime/edit", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "09:00–12:30") {
		t.Errorf("expected updated session row, got:\n%s", res.Body.String())
	}
}
