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
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStyleguideRouteRendersBehindAuth(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)

	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure:  usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// Unauthenticated → redirect to login.
	noRedir := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedir.Get(ts.URL + "/ui")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("unauth /ui = %d, want 302", res.StatusCode)
	}

	// Authenticated → 200 + showcase content.
	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", ts.URL+"/ui", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("auth /ui = %d, want 200", res2.StatusCode)
	}
	body := make([]byte, 64*1024)
	n, _ := res2.Body.Read(body)
	out := string(body[:n])
	for _, w := range []string{"Design-System", "/static/app.css", "data-theme-toggle"} {
		if !strings.Contains(out, w) {
			t.Errorf("/ui body missing %q", w)
		}
	}
}
