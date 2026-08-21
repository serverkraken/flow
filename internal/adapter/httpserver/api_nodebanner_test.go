package httpserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// newAPIBannerServer mirrors newAPILogoServer for the banner route: this is
// the path agents use (flow-mcp, CLI) now that identity in the WebUI is
// monogram + colour and the image lives in the banner slot.
func newAPIBannerServer(t *testing.T) (*httpserver.Server, *testutil.FakeNodeStore, *testutil.FakeNodeBannerStore, *sse.Bus) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	emitter := sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk)
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBannerStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, testutil.NewFakeNodeLogoStore(), bs, testutil.NewFakeTagStore())
	users := testutil.NewFakeUserStore()
	u, err := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)

	srv := &httpserver.Server{
		Verifier:         testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:           usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:              bus,
		Emitter:          emitter,
		Clock:            clk,
		Session:          codec,
		Users:            users,
		UploadNodeBanner: usecase.UploadNodeBanner{Nodes: ns, Banners: bs, Aggregate: agg, Clock: clk},
	}
	return srv, ns, bs, bus
}

func putBanner(t *testing.T, ts *httptest.Server, nodeID string, data []byte) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	return doArtifact(t, ts, http.MethodPut, "/api/v1/nodes/"+nodeID+"/banner", body)
}

func TestHandleAPISetNodeBanner_HappyPath_StampsRefBlobAndEmits(t *testing.T) {
	srv, ns, bs, bus := newAPIBannerServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "u1")
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := putBanner(t, ts, "n1", pngPixelBytes(t))
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got domain.Node
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "n1" || got.BannerRef == "" {
		t.Errorf("got id=%q bannerRef=%q, want n1 with non-empty ref", got.ID, got.BannerRef)
	}
	stored, err := bs.Get(context.Background(), "u1", "n1")
	if err != nil || stored.Ref != got.BannerRef {
		t.Errorf("blob and ref diverged: %+v err=%v", stored, err)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventNodeUpdated {
			t.Errorf("event type = %q, want %q", ev.Type, domain.EventNodeUpdated)
		}
	default:
		t.Error("no event emitted, want node.updated")
	}
}

func TestHandleAPISetNodeBanner_BadTypeUnknownNodeAndBadBase64(t *testing.T) {
	srv, ns, _, _ := newAPIBannerServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "u1")

	res := putBanner(t, ts, "n1", []byte("<svg onload=alert(1)></svg>"))
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("svg upload = %d, want 400", res.StatusCode)
	}

	res2 := putBanner(t, ts, "gibtsnicht", pngPixelBytes(t))
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown node = %d, want 404", res2.StatusCode)
	}

	res3 := doArtifact(t, ts, http.MethodPut, "/api/v1/nodes/n1/banner", []byte(`{"dataBase64":"nicht base64!!"}`))
	_ = res3.Body.Close()
	if res3.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid base64 = %d, want 400", res3.StatusCode)
	}
}
