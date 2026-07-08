package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newNodeMRUServer() (*httpserver.Server, *testutil.FakeSessionStore) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	ids := &testutil.FakeIDGen{}
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(), Clock: clk,
		NodeMRU: usecase.NodeMRU{Sessions: sessions},
	}
	return srv, sessions
}

func TestHandleNodeMRU_ShapeAndAuth(t *testing.T) {
	srv, sessions := newNodeMRUServer()
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mk := func(id, node string, start time.Time) {
		stop := start.Add(time.Hour)
		_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: "id-1", NodeID: &node, Start: start, Stop: &stop})
	}
	mk("a", "n-old", base)
	mk("b", "n-new", base.AddDate(0, 0, 10))

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// unauth → 401
	res, _ := http.Get(ts.URL + "/api/v1/nodes/mru")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth want 401, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/nodes/mru", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var out []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if len(out) != 2 || out[0]["nodeId"] != "n-new" || out[1]["nodeId"] != "n-old" {
		t.Fatalf("want newest-first [n-new, n-old], got %v", out)
	}
	if _, ok := out[0]["lastBookedAt"].(string); !ok {
		t.Errorf("row must carry lastBookedAt string, got %v", out[0])
	}
}

func TestHandleNodeMRU_EmptyArray(t *testing.T) {
	srv, _ := newNodeMRUServer()
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/nodes/mru", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, _ := http.DefaultClient.Do(req)
	defer func() { _ = res.Body.Close() }()
	var out []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("no bookings → empty array, got %v", out)
	}
}
