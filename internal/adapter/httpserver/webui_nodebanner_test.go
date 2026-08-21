package httpserver_test

import (
	"bytes"
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

// newWebNodeBannerServer wires the banner serving path over the fake stores.
func newWebNodeBannerServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeBannerStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Users:         users,
		Session:       codec,
		Bus:           bus,
		Emitter:       sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:         clk,
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		ListNodes:     usecase.ListNodes{Nodes: ns},
		GetNode:       usecase.GetNode{Nodes: ns},
		GetNodeBanner: usecase.GetNodeBanner{Banners: bs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, bs
}

// TestWebNodeBanner_ServesBlobWithImmutableCaching mirrors the logo path: the
// <img> URL carries ?v={BannerRef}, so each URL's content never changes —
// strong ETag plus long-lived PRIVATE caching (a banner is user content, a
// shared cache must not hand it to the next tenant), and If-None-Match
// short-circuits to 304.
func TestWebNodeBanner_ServesBlobWithImmutableCaching(t *testing.T) {
	ts, cookie, bs := newWebNodeBannerServer(t)
	ctx := context.Background()
	png := pngPixel(t)
	if err := bs.Put(ctx, domain.NodeBanner{
		NodeID: "n1", OwnerID: "u1", Mime: "image/png", Ref: "abc123abc123",
		Bytes: png, UpdatedAt: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	get := func(ifNoneMatch string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+"/nodes/n1/banner?v=abc123abc123", nil)
		req.AddCookie(cookie)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := get("")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, png) {
		t.Errorf("served %d bytes, want the stored image", len(body))
	}
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type=%q want image/png", got)
	}
	if got := res.Header.Get("ETag"); got != `"abc123abc123"` {
		t.Errorf("ETag=%q want the content hash", got)
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "private") || !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control=%q must be private and immutable", cc)
	}

	res2 := get(`"abc123abc123"`)
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match: status %d, want 304", res2.StatusCode)
	}
	if res2.Header.Get("ETag") != `"abc123abc123"` {
		t.Errorf("a 304 must still carry the ETag, got %q", res2.Header.Get("ETag"))
	}
}

// TestWebNodeBanner_ForeignAndMissingAre404 keeps the banner owner-scoped and
// tells a foreign owner nothing beyond "not here".
func TestWebNodeBanner_ForeignAndMissingAre404(t *testing.T) {
	ts, cookie, bs := newWebNodeBannerServer(t)
	if err := bs.Put(context.Background(), domain.NodeBanner{
		NodeID: "fremd", OwnerID: "u-fremd", Mime: "image/png", Ref: "ffff",
		Bytes: pngPixel(t), UpdatedAt: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"fremd", "gibtsnicht"} {
		req, _ := http.NewRequest("GET", ts.URL+"/nodes/"+id+"/banner", nil)
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("GET /nodes/%s/banner = %d, want 404", id, res.StatusCode)
		}
	}
}
