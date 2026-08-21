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

// newNavSrv wires just enough for GET /ui/nav/tree: the handler degrades
// gracefully when documents and running sessions are absent.
func newNavSrv(t *testing.T) (*httptest.Server, *testutil.FakeNodeStore, string) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	nodes := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "msoent@x.de", "Soenne")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	srv := &httpserver.Server{
		Users:     users,
		Session:   codec,
		Verifier:  testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:    usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:       sse.NewBus(),
		Clock:     clk,
		ListNodes: usecase.ListNodes{Nodes: nodes},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookieVal, _ := codec.Issue("u1")
	return ts, nodes, cookieVal
}

func navBody(t *testing.T, ts *httptest.Server, cookieVal string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/ui/nav/tree?active=wissen", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("nav fragment status %d, want 200", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestNavTreeFragment_NamesNotSlugs pins an agreed deviation from the K3Rail
// mockup (Spec Teil A): the rail calls every register by its NAME, including
// repos and branches, which the mockup renders as slug-ish paths. The
// deviation is decided, so it needs a guard — the slug swap it replaced lived
// in this very handler.
func TestNavTreeFragment_NamesNotSlugs(t *testing.T) {
	ts, nodes, cookieVal := newNavSrv(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	eng := "e1"
	vor := "v1"
	mk := func(id, name, slug string, kind domain.NodeKind, parent *string) {
		n, err := domain.NewNode(id, "u1", name, slug, now)
		if err != nil {
			t.Fatal(err)
		}
		n.Kind = kind
		n.ParentID = parent
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	mk(eng, "Privat", "privat", domain.KindEngagement, nil)
	mk(vor, "CI-Plattform", "ci-plattform", domain.KindVorhaben, &eng)
	mk("r1", "flow", "flow-repo-slug", domain.KindRepo, &vor)
	mk("b1", "feat-l7", "feat-l7-branch-slug", domain.KindBranch, &vor)

	body := navBody(t, ts, cookieVal)

	for _, want := range []string{"Privat", "CI-Plattform", ">flow</span>", ">feat-l7</span>"} {
		if !strings.Contains(body, want) {
			t.Errorf("nav fragment missing %q in:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"flow-repo-slug", "feat-l7-branch-slug", "ci-plattform"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("nav row fell back to the slug %q:\n%s", unwanted, body)
		}
	}
}

// TestNavTreeFragment_RepoRowsAreMonoButNotGrey pins the 2026-08-17 spec
// addendum: the bottom level is carried by the type FAMILY, not by lightness.
// Mono stays (TOKENS.md reserves JetBrains Mono for what is measured and
// ADDRESSED — a repo name is an address), text-muted does not, and the level
// square that an earlier draft wanted here was dropped for good: the caret
// moved out of the text flow into the indent gutter, so there is no slot left.
func TestNavTreeFragment_RepoRowsAreMonoButNotGrey(t *testing.T) {
	ts, nodes, cookieVal := newNavSrv(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	eng := "e1"
	for _, spec := range []struct {
		id, name, slug string
		kind           domain.NodeKind
		parent         *string
	}{
		{"e1", "Privat", "privat", domain.KindEngagement, nil},
		{"r1", "flow", "flow-repo", domain.KindRepo, &eng},
	} {
		n, err := domain.NewNode(spec.id, "u1", spec.name, spec.slug, now)
		if err != nil {
			t.Fatal(err)
		}
		n.Kind = spec.kind
		n.ParentID = spec.parent
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	body := navBody(t, ts, cookieVal)

	if !strings.Contains(body, "font-mono text-[11.5px]") {
		t.Errorf("repo row lost its mono type:\n%s", body)
	}
	if strings.Contains(body, "font-mono text-[11.5px] text-muted") {
		t.Errorf("repo row must not be greyed out — the family carries the level, not the lightness:\n%s", body)
	}
	if strings.Contains(body, "nv-sq") {
		t.Errorf("the level square was dropped with the caret's move into the gutter:\n%s", body)
	}
}
