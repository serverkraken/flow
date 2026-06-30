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

// TestHomeHome_RendersLanding verifies that GET / returns 200 and renders
// the Home heading plus section links for Zeit (/zeit), Wissen (/wissen),
// and Projekte (/nodes).
func TestHomeHome_RendersLanding(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)

	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure:  usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
	}

	// Unauthenticated → redirect to login (webAuth gate).
	noRedir := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	res, err := noRedir.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("unauth GET / = %d, want 302", res.StatusCode)
	}

	// Authenticated → 200 with Home heading + three section links.
	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Home",           // heading — nav.home key renders "Home"
		`href="/zeit"`,   // Zeit section link
		`href="/wissen"`, // Wissen section link
		`href="/nodes"`,  // Projekte section link
		"/static/app.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / missing %q", want)
		}
	}
}
