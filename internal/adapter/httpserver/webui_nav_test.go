package httpserver_test

import (
	"context"
	"errors"
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
func newNavTreeServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore, *testutil.FakeSessionStore, testutil.FakeClock) {
	t.Helper()
	ss := testutil.NewFakeSessionStore()
	ts, cookie, ns, clk := newNavTreeServerWith(t, ss)
	return ts, cookie, ns, ss, clk
}

// newNavTreeServerWith wires the nav-tree fragment server around an arbitrary
// SessionStore implementation so tests can inject session-listing failures.
func newNavTreeServerWith(t *testing.T, ss ports.SessionStore) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore, testutil.FakeClock) {
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
		ListNodes:    usecase.ListNodes{Nodes: ns},
		ListSessions: usecase.ListSessions{Sessions: ss, Clock: clk},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns, clk
}

// failingSessionStore embeds the fake store but always fails List — lets the
// store-error test drive the handler's warn-only branch with a real error.
type failingSessionStore struct {
	*testutil.FakeSessionStore
}

func (f *failingSessionStore) List(context.Context, string, time.Time) ([]domain.WorkSession, error) {
	return nil, errors.New("session store down")
}

// TestNavTreeFragment_Returns200WithNodeLink verifies that GET /ui/nav/tree
// returns 200 and renders a link to /nodes/{id} for a seeded engagement.
func TestNavTreeFragment_Returns200WithNodeLink(t *testing.T) {
	ts, cookie, ns, _, _ := newNavTreeServer(t)

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
	ts, cookie, _, _, _ := newNavTreeServer(t)

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
	ts, _, _, _, _ := newNavTreeServer(t)

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

// TestNavTreeFragment_WithHourBadges verifies that seeded sessions render as
// hour badges in the tree (subtree aggregation + ancestor rollup).
func TestNavTreeFragment_WithHourBadges(t *testing.T) {
	ts, cookie, ns, ss, _ := newNavTreeServer(t)

	// Seed nodes: e1 -> v1 -> r1.
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	e1, _ := domain.NewNode("e1", "u1", "Engagement", "eng", now)
	e1.Kind = domain.KindEngagement
	e1.Status = domain.NodeActive
	if _, err := ns.Create(context.Background(), e1); err != nil {
		t.Fatalf("Create e1: %v", err)
	}

	v1, _ := domain.NewNode("v1", "u1", "Vorhaben", "vor", now)
	v1.Kind = domain.KindVorhaben
	v1.ParentID = &e1.ID
	v1.Status = domain.NodeActive
	if _, err := ns.Create(context.Background(), v1); err != nil {
		t.Fatalf("Create v1: %v", err)
	}

	r1, _ := domain.NewNode("r1", "u1", "Repo", "repo", now)
	r1.Kind = domain.KindRepo
	r1.ParentID = &v1.ID
	r1.Status = domain.NodeActive
	if _, err := ns.Create(context.Background(), r1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}

	// Seed sessions: 2h on r1, 1h on v1 (unbooked ignored).
	s1, _ := domain.NewWorkSession("s1", "u1", &r1.ID, now.Add(-2*time.Hour))
	s1.Stop = &now
	if _, err := ss.Create(context.Background(), s1); err != nil {
		t.Fatalf("Create s1: %v", err)
	}

	s2, _ := domain.NewWorkSession("s2", "u1", &v1.ID, now.Add(-1*time.Hour))
	s2.Stop = &now
	if _, err := ss.Create(context.Background(), s2); err != nil {
		t.Fatalf("Create s2: %v", err)
	}

	s3, _ := domain.NewWorkSession("s3", "u1", nil, now.Add(-30*time.Minute))
	s3.Stop = &now
	if _, err := ss.Create(context.Background(), s3); err != nil {
		t.Fatalf("Create s3: %v", err)
	}

	// GET /ui/nav/tree.
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

	// e1 should show "3h" (2h from r1 + 1h from v1).
	// v1 should show "3h" (1h direct + 2h from r1 subtree).
	// r1 should show "2h" (direct).
	// Badges under 1h stay empty, so s3 (unbooked) doesn't affect rendering.
	if !strings.Contains(body, "Engagement") || !strings.Contains(body, "3h") {
		t.Errorf("nav tree missing engagement or 3h badge in:\n%s", body)
	}
	if !strings.Contains(body, "Repo") || !strings.Contains(body, "2h") {
		t.Errorf("nav tree missing repo or 2h badge in:\n%s", body)
	}
}

// TestNavTreeFragment_StoreError_StillRenders injects a session store whose
// List always errors and verifies the handler's warn-only branch: the tree
// renders with full structure (200, link, name) and no hour badge appears.
func TestNavTreeFragment_StoreError_StillRenders(t *testing.T) {
	fss := &failingSessionStore{FakeSessionStore: testutil.NewFakeSessionStore()}
	ts, cookie, ns, clk := newNavTreeServerWith(t, fss)

	// Seed an active node.
	now := clk.T
	e1, _ := domain.NewNode("e1", "u1", "Engagement", "eng", now)
	e1.Kind = domain.KindEngagement
	e1.Status = domain.NodeActive
	if _, err := ns.Create(context.Background(), e1); err != nil {
		t.Fatalf("Create e1: %v", err)
	}

	// Seed a 2h session on it into the embedded fake: if List did NOT fail,
	// the fragment would carry a "2h" badge — its absence below proves the
	// error branch really ran.
	s1, _ := domain.NewWorkSession("s1", "u1", &e1.ID, now.Add(-2*time.Hour))
	s1.Stop = &now
	if _, err := fss.Create(context.Background(), s1); err != nil {
		t.Fatalf("Create s1: %v", err)
	}

	// GET /ui/nav/tree.
	req, _ := http.NewRequest("GET", ts.URL+"/ui/nav/tree", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/nav/tree (failing session store) = %d, want 200", resp.StatusCode)
	}

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

	// Tree structure must render despite the session-store error.
	if !strings.Contains(body, "Engagement") || !strings.Contains(body, `href="/nodes/e1"`) {
		t.Errorf("nav tree missing engagement node structure in:\n%s", body)
	}
	// No badge may appear: neither the would-be "2h" text nor the badge span
	// (tnum is only used by the hour badge in this fragment).
	if strings.Contains(body, ">2h<") || strings.Contains(body, "tnum") {
		t.Errorf("hour badge must not render when session listing fails:\n%s", body)
	}
}
