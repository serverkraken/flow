package httpserver_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
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

// pngPixel is a valid 1x1 PNG (sniffs as image/png) — mirrors the helper in
// internal/usecase/node_logo_test.go; cross-package test helpers aren't
// importable, so it's duplicated here.
func pngPixel(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newWebNodeLogoServer builds a test server with the node + node-logo
// usecases wired (mirrors newWebNodesServerFull in webui_nodes_test.go,
// extended with the Upload/Delete/GetNodeLogo usecases under test). It
// returns the live *httpserver.Server itself (so the test can call
// UploadNodeLogo.Execute directly, the same way the usecase tests seed a
// logo), the httptest server, the cookie-authed session for user "u1", and
// the fake node store for seeding.
func newWebNodeLogoServer(t *testing.T) (*httpserver.Server, *httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, ls, tags)
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     bus,
		Emitter: sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:   clk,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   ids,
			Allow: func(ports.Identity) bool { return true },
		},
		CreateNode:     usecase.CreateNode{Nodes: ns, Aggregate: agg, IDs: ids, Clock: clk},
		ListNodes:      usecase.ListNodes{Nodes: ns},
		GetNode:        usecase.GetNode{Nodes: ns},
		UploadNodeLogo: usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk},
		DeleteNodeLogo: usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk},
		GetNodeLogo:    usecase.GetNodeLogo{Logos: ls},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return srv, ts, &http.Cookie{Name: "flow_session", Value: cv}, ns
}

// authedGet performs a cookie-authed GET, optionally setting one request
// header (name, value) — mirrors postN's cookie-request-building pattern in
// webui_nodes_test.go, adapted to return the full *http.Response so tests can
// inspect caching headers.
func authedGet(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) *http.Response {
	t.Helper()
	return authedGetWithHeader(t, ts, c, path, "", "")
}

// authedGetWithHeader is authedGet plus an optional extra request header
// (used for If-None-Match conditional requests).
func authedGetWithHeader(t *testing.T, ts *httptest.Server, c *http.Cookie, path, headerName, headerValue string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(c)
	if headerName != "" {
		req.Header.Set(headerName, headerValue)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestHandleWebNodeLogo_ServeETag304 pins GET /nodes/{id}/logo: 200 with
// Content-Type/ETag/Cache-Control + exact bytes, 304 on a matching
// If-None-Match, and 404 for a node with no uploaded logo.
func TestHandleWebNodeLogo_ServeETag304(t *testing.T) {
	srv, ts, c, ns := newWebNodeLogoServer(t)

	ctx := context.Background()
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", time.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	png := pngPixel(t)
	if _, err := srv.UploadNodeLogo.Execute(ctx, "u1", "n1", png); err != nil {
		t.Fatal(err)
	}

	res := authedGet(t, ts, c, "/nodes/n1/logo")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type %q", ct)
	}
	etag := res.Header.Get("ETag")
	if etag == "" || res.Header.Get("Cache-Control") == "" {
		t.Errorf("missing caching headers: etag=%q cc=%q", etag, res.Header.Get("Cache-Control"))
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !bytes.Equal(body, png) {
		t.Error("served bytes differ from upload")
	}

	res2 := authedGetWithHeader(t, ts, c, "/nodes/n1/logo", "If-None-Match", etag)
	if res2.StatusCode != http.StatusNotModified {
		t.Errorf("conditional GET → %d, want 304", res2.StatusCode)
	}
	_ = res2.Body.Close()

	res3 := authedGet(t, ts, c, "/nodes/ghost/logo")
	if res3.StatusCode != http.StatusNotFound {
		t.Errorf("missing logo → %d, want 404", res3.StatusCode)
	}
	_ = res3.Body.Close()
}

// TestHandleWebNodeLogo_OwnerScoped pins the binding constraint that serving
// is owner-scoped: a second user's session must 404 on the first user's
// node/logo, not leak the blob.
func TestHandleWebNodeLogo_OwnerScoped(t *testing.T) {
	srv, ts, _, ns := newWebNodeLogoServer(t)

	ctx := context.Background()
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", time.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	if _, err := srv.UploadNodeLogo.Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}

	// Seed a second user and issue their own session cookie.
	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x.de", "O")
	_, _ = srv.Users.UpsertBySub(ctx, u2)
	cv2, _ := srv.Session.Issue("u2")
	c2 := &http.Cookie{Name: "flow_session", Value: cv2}

	res := authedGet(t, ts, c2, "/nodes/n1/logo")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("foreign user GET = %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()
}
