package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestAllRoutesRegistered(t *testing.T) {
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "s"}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		Users:         users,
		Session:       websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour),
		OIDCAuth:      fakeAuth{url: "https://id/authorize?state="},
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateNode: usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:  usecase.ListNodes{Nodes: ps},
	}
	h := srv.Routes()
	cases := []struct{ method, path string }{
		{"GET", "/healthz"},
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/events"},
		{"POST", "/api/v1/sessions"},
		{"POST", "/api/v1/sessions/x/stop"},
		{"PATCH", "/api/v1/sessions/x"},
		{"DELETE", "/api/v1/sessions/x"},
		{"GET", "/api/v1/sessions"},
		{"POST", "/api/v1/nodes"},
		{"GET", "/api/v1/nodes"},
		{"GET", "/api/v1/export"},
		{"POST", "/api/v1/nodes/x/rate"},
		{"POST", "/api/v1/documents"},
		{"GET", "/api/v1/documents"},
		{"GET", "/api/v1/documents/x"},
		{"PUT", "/api/v1/documents/x"},
		{"DELETE", "/api/v1/documents/x"},
		{"GET", "/api/v1/documents/x/backlinks"},
		{"GET", "/auth/login"},
		{"GET", "/auth/callback"},
		{"POST", "/auth/logout"},
		{"GET", "/zeit"},
		{"GET", "/ui/worktime"},
		{"POST", "/ui/worktime/start"},
		{"POST", "/ui/worktime/stop"},
		{"POST", "/ui/worktime/add"},
		{"POST", "/ui/worktime/edit"},
		{"POST", "/ui/worktime/delete"},
		{"GET", "/ui/timer"},
		{"GET", "/ui/timer/chip"},
		{"POST", "/ui/timer/start"},
		{"POST", "/ui/timer/stop"},
		{"POST", "/ui/timer/switch"},
		{"GET", "/static/app.css"},
		{"GET", "/wissen"},
		{"GET", "/ui/wissen/list"},
		{"GET", "/wissen/daily"},
		{"GET", "/wissen/projekte"},
		{"GET", "/wissen/frei"},
		{"GET", "/wissen/system"},
		{"GET", "/ui/wissen/list/daily"},
		{"GET", "/wissen/neu"},
		{"POST", "/wissen/preview"},
		{"POST", "/wissen"},
		{"GET", "/wissen/x"},
		{"GET", "/wissen/x/bearbeiten"},
		{"POST", "/wissen/x"},
		{"POST", "/wissen/x/delete"},
		{"POST", "/wissen/x/reembed"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s is not registered (404)", tc.method, tc.path)
		}
	}
}
