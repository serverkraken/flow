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

// newAPILogoServer wires a Server with just enough usecases for the REST
// logo route (mirrors newArtifactServer, swapping the artifact usecases for
// UploadNodeLogo). The authenticated user is a fixed "u1" behind the
// Bearer-token FakeVerifier.
func newAPILogoServer(t *testing.T) (*httpserver.Server, *testutil.FakeNodeStore, *sse.Bus) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	emitter := sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk)
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(ns, ls, testutil.NewFakeNodeBannerStore(), tags)
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
		Verifier:       testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:         usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:            bus,
		Emitter:        emitter,
		Clock:          clk,
		Session:        codec,
		Users:          users,
		UploadNodeLogo: usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Aggregate: agg, Clock: clk},
	}
	return srv, ns, bus
}

func putLogo(t *testing.T, ts *httptest.Server, nodeID string, data []byte) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	return doArtifact(t, ts, http.MethodPut, "/api/v1/nodes/"+nodeID+"/logo", body)
}

func TestHandleAPISetNodeLogo_HappyPath_StampsRefAndEmits(t *testing.T) {
	srv, ns, bus := newAPILogoServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "u1")
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := putLogo(t, ts, "n1", pngPixelBytes(t))
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got domain.Node
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "n1" || got.LogoRef == "" {
		t.Errorf("got id=%q logoRef=%q, want n1 with non-empty ref", got.ID, got.LogoRef)
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

func TestHandleAPISetNodeLogo_BadType_Rejects400(t *testing.T) {
	srv, ns, _ := newAPILogoServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "u1")

	res := putLogo(t, ts, "n1", []byte("plain text, not an image"))
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestHandleAPISetNodeLogo_UnknownNode_404(t *testing.T) {
	srv, _, _ := newAPILogoServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res := putLogo(t, ts, "ghost", pngPixelBytes(t))
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestHandleAPISetNodeLogo_InvalidBase64_400(t *testing.T) {
	srv, ns, _ := newAPILogoServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "u1")

	body, _ := json.Marshal(map[string]string{"dataBase64": "%%% not base64 %%%"})
	res := doArtifact(t, ts, http.MethodPut, "/api/v1/nodes/n1/logo", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}
