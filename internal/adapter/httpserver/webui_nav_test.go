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

// newNavTreeServer builds a minimal test server wired with only the usecases
// needed for the nav-tree fragment endpoint.
func newNavTreeServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   ids,
			Allow: func(ports.Identity) bool { return true },
		},
		ListNodes: usecase.ListNodes{Nodes: ns},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns
}

// TestNavTreeFragment_Returns200WithNodeLink verifies that GET /ui/nav/tree
// returns 200 and renders a link to /nodes/{id} for a seeded engagement.
func TestNavTreeFragment_Returns200WithNodeLink(t *testing.T) {
	ts, cookie, ns := newNavTreeServer(t)

	// Seed an active engagement node.
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	eng, err := domain.NewNode("eng-1", "u1", "Kunden A", "kunden-a", now)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	eng.Kind = domain.KindEngagement
	eng.Status = domain.NodeActive
	if _, err := ns.Create(context.Background(), eng); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed an archived engagement — must NOT appear in the tree.
	arch, _ := domain.NewNode("eng-2", "u1", "Alt", "alt", now)
	arch.Kind = domain.KindEngagement
	arch.Status = domain.NodeArchived
	_, _ = ns.Create(context.Background(), arch)

	// GET /ui/nav/tree — authenticated.
	req, _ := http.NewRequest("GET", ts.URL+"/ui/nav/tree", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/nav/tree = %d, want 200", resp.StatusCode)
	}

	// Must contain a link to the active engagement node.
	buf := new(strings.Builder)
	b := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(b)
		buf.Write(b[:n])
		if rerr != nil {
			break
		}
	}
	body := buf.String()

	wantLink := `href="/nodes/eng-1"`
	if !strings.Contains(body, wantLink) {
		t.Errorf("GET /ui/nav/tree body missing %q\n%s", wantLink, body)
	}
	wantName := "Kunden A"
	if !strings.Contains(body, wantName) {
		t.Errorf("GET /ui/nav/tree body missing node name %q\n%s", wantName, body)
	}
	// Archived node must NOT appear.
	if strings.Contains(body, "Alt") {
		t.Errorf("GET /ui/nav/tree must not render archived nodes; body=%.500s", body)
	}
}

// TestNavTreeFragment_EmptyReturns200 verifies 200 + empty-state when no nodes.
func TestNavTreeFragment_EmptyReturns200(t *testing.T) {
	ts, cookie, _ := newNavTreeServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/ui/nav/tree", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/nav/tree (empty) = %d, want 200", resp.StatusCode)
	}
}

// TestNavTreeFragment_UnauthRedirects verifies the webAuth gate protects the endpoint.
func TestNavTreeFragment_UnauthRedirects(t *testing.T) {
	ts, _, _ := newNavTreeServer(t)

	noRedir := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedir.Get(ts.URL + "/ui/nav/tree")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unauth GET /ui/nav/tree = %d, want 302", resp.StatusCode)
	}
}
