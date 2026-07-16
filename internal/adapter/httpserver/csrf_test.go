package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

const csrfTestOrigin = "https://flow.example.com"

func csrfTestServer(t *testing.T) (*Server, *http.Cookie, domain.User) {
	t.Helper()
	users := testutil.NewFakeUserStore()
	u, err := domain.NewUser("owner-a", "sub-a", "alice", "alice@example.com", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	value, err := codec.Issue(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		PublicBaseURL: csrfTestOrigin,
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-a", Username: u.Username}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Users:         users,
		Session:       codec,
	}, &http.Cookie{Name: sessionCookie, Value: value}, u
}

func TestWebAuthProtectsCookieWritesUsingConfiguredPublicOrigin(t *testing.T) {
	srv, cookie, user := csrfTestServer(t)
	var gotUser domain.User
	var gotActor actor.Actor
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = userFrom(r.Context())
		gotActor = actor.FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	h := srv.webAuth(probe)

	tests := []struct {
		name       string
		origin     string
		referer    string
		host       string
		forwarded  string
		wantStatus int
	}{
		{name: "trusted origin behind proxy", origin: csrfTestOrigin, host: "flow-server.flow.svc:8080", forwarded: "attacker.invalid", wantStatus: http.StatusNoContent},
		{name: "trusted referer fallback", referer: csrfTestOrigin + "/wissen", host: "internal:8080", wantStatus: http.StatusNoContent},
		{name: "cross origin", origin: "https://evil.example", host: "flow.example.com", forwarded: "flow.example.com", wantStatus: http.StatusForbidden},
		{name: "missing source", host: "flow.example.com", forwarded: "flow.example.com", wantStatus: http.StatusForbidden},
		{name: "opaque origin", origin: "null", host: "flow.example.com", wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://internal/mutate", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			req.Header.Set("X-Forwarded-Host", tt.forwarded)
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("X-Flow-Actor", "mallory-agent")
			req.AddCookie(cookie)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
	if gotUser.ID != user.ID {
		t.Fatalf("authenticated owner = %q, want %q", gotUser.ID, user.ID)
	}
	if gotActor != (actor.Actor{Kind: actor.Human, Ref: user.DisplayName}) {
		t.Fatalf("cookie actor = %+v, want authenticated human", gotActor)
	}
}

func TestWebAuthLeavesSafeCookieReadsOriginIndependent(t *testing.T) {
	srv, cookie, _ := csrfTestServer(t)
	h := srv.webAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "http://internal/read", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("safe read status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestAuthAnyRequiresOriginOnlyForCookieAuthenticatedWrites(t *testing.T) {
	srv, cookie, _ := csrfTestServer(t)
	h := srv.authAny(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	bearerReq := httptest.NewRequest(http.MethodPost, "http://internal/write", nil)
	bearerReq.Header.Set("Authorization", "Bearer valid")
	bearerRR := httptest.NewRecorder()
	h.ServeHTTP(bearerRR, bearerReq)
	if bearerRR.Code != http.StatusNoContent {
		t.Fatalf("bearer write status = %d, want %d", bearerRR.Code, http.StatusNoContent)
	}

	cookieReq := httptest.NewRequest(http.MethodPost, "http://internal/write", nil)
	cookieReq.AddCookie(cookie)
	cookieRR := httptest.NewRecorder()
	h.ServeHTTP(cookieRR, cookieReq)
	if cookieRR.Code != http.StatusForbidden {
		t.Fatalf("source-less cookie write status = %d, want %d", cookieRR.Code, http.StatusForbidden)
	}

	cookieReq = httptest.NewRequest(http.MethodPost, "http://internal/write", nil)
	cookieReq.Header.Set("Origin", csrfTestOrigin)
	cookieReq.AddCookie(cookie)
	cookieRR = httptest.NewRecorder()
	h.ServeHTTP(cookieRR, cookieReq)
	if cookieRR.Code != http.StatusNoContent {
		t.Fatalf("same-origin cookie write status = %d, want %d", cookieRR.Code, http.StatusNoContent)
	}
}

func TestLogoutRouteRejectsCrossOriginCookieRequest(t *testing.T) {
	srv, cookie, _ := csrfTestServer(t)
	req := httptest.NewRequest(http.MethodPost, csrfTestOrigin+"/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("logout status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}
