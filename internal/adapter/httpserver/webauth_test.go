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

type fakeAuth struct {
	url string
	id  ports.Identity
}

func (f fakeAuth) AuthCodeURL(string) string                                { return f.url }
func (f fakeAuth) Exchange(context.Context, string) (ports.Identity, error) { return f.id, nil }

func TestAuthCodeFlowSetsSessionCookie(t *testing.T) {
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Ensure:   usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(),
		Users:    users,
		OIDCAuth: fakeAuth{url: "https://id/authorize?state=", id: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Session:  websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour),
		Dev:      true,
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := client.Get(ts.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login status %d", res.StatusCode)
	}
	var state *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "flow_oidc_state" {
			state = c
		}
	}
	if state == nil {
		t.Fatal("no state cookie set on login")
	}

	req, _ := http.NewRequest("GET", ts.URL+"/auth/callback?code=abc&state="+state.Value, nil)
	req.AddCookie(state)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
		t.Fatalf("callback status %d loc %q", res.StatusCode, res.Header.Get("Location"))
	}
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "flow_session" {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("no session cookie set on callback")
	}

	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/events", nil)
	req.AddCookie(session)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res, err = http.DefaultClient.Do(req.WithContext(ctx))
	if err == nil && res.StatusCode != http.StatusOK {
		t.Fatalf("SSE with session cookie status %d, want 200", res.StatusCode)
	}
}

func TestWebHomeRendersWithSessionCookie(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		Users:         users,
		Session:       codec,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("home status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "start timer") {
		t.Fatalf("home did not render the worktime screen:\n%s", string(body))
	}
}
