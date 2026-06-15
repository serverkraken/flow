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
	ps := testutil.NewFakeProjectStore()
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
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
	}
	h := srv.Routes()
	cases := []struct{ method, path string }{
		{"GET", "/healthz"},
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/events"},
		{"POST", "/api/v1/sessions"},
		{"POST", "/api/v1/sessions/x/stop"},
		{"GET", "/api/v1/sessions"},
		{"POST", "/api/v1/projects"},
		{"GET", "/api/v1/projects"},
		{"GET", "/api/v1/export"},
		{"POST", "/api/v1/projects/x/rate"},
		{"POST", "/api/v1/documents"},
		{"GET", "/api/v1/documents"},
		{"GET", "/api/v1/documents/x"},
		{"PUT", "/api/v1/documents/x"},
		{"DELETE", "/api/v1/documents/x"},
		{"GET", "/api/v1/documents/x/backlinks"},
		{"GET", "/auth/login"},
		{"GET", "/auth/callback"},
		{"POST", "/auth/logout"},
		{"GET", "/"},
		{"GET", "/ui/worktime"},
		{"POST", "/ui/worktime/start"},
		{"POST", "/ui/worktime/stop"},
		{"GET", "/static/app.css"},
		{"GET", "/docs"},
		{"GET", "/ui/docs/list"},
		{"GET", "/docs/new"},
		{"POST", "/docs"},
		{"GET", "/docs/x"},
		{"GET", "/docs/x/edit"},
		{"POST", "/docs/x"},
		{"POST", "/docs/x/delete"},
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
