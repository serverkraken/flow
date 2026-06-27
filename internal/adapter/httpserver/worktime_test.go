package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func newWorktimeServer(t *testing.T) (*httpserver.Server, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	ids := &testutil.FakeIDGen{}
	return &httpserver.Server{
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               sse.NewBus(),
		Clock:             clk,
		Dev:               true,
		StartSession:      usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessions:      usecase.ListSessions{Sessions: sessions, Clock: clk},
		AddSession:        usecase.AddSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: sessions},
		EditSession:       usecase.EditSession{Sessions: sessions},
		ListSessionsPage:  usecase.ListSessionsPage{Sessions: sessions},
	}, sessions
}

func authPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return res
}

func TestBackfillSession_HappyAndList(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Nachbuchen 09:00–12:00 on 2026-06-15 (clock is 18:00 same day).
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T12:00:00Z", "tag": "deep",
	})
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("backfill status = %d (%s)", res.StatusCode, b)
	}
	var created domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&created)
	_ = res.Body.Close()
	if created.Stop == nil || created.Tag != "deep" {
		t.Fatalf("backfill result wrong: %+v", created)
	}

	// GET with since+until brackets the day → returns the session.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/sessions?since=2026-06-15T00:00:00Z&until=2026-06-16T00:00:00Z", nil)
	req.Header.Set("Authorization", "Bearer x")
	res2, _ := http.DefaultClient.Do(req)
	var list []domain.WorkSession
	_ = json.NewDecoder(res2.Body).Decode(&list)
	_ = res2.Body.Close()
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("range list = %+v, want the backfilled session", list)
	}
}

func TestBackfillSession_FutureRejected(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T19:00:00Z", "stop": "2026-06-15T20:00:00Z", // after 18:00 clock
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("future backfill status = %d, want 400", res.StatusCode)
	}
}

func TestBackfillSession_OverlapConflict(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	first := map[string]any{"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T11:00:00Z"}
	if r := authPost(t, ts.URL+"/api/v1/sessions", first); r.StatusCode != http.StatusCreated {
		t.Fatalf("seed backfill status = %d", r.StatusCode)
	}
	overlap := map[string]any{"start": "2026-06-15T10:00:00Z", "stop": "2026-06-15T12:00:00Z"}
	res := authPost(t, ts.URL+"/api/v1/sessions", overlap)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("overlap backfill status = %d, want 409", res.StatusCode)
	}
}

func TestBackfillSession_MixedTimestamps400(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"start": "2026-06-15T09:00:00Z"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("mixed timestamps status = %d, want 400", res.StatusCode)
	}
}

func TestBackfillSession_StopOnly400(t *testing.T) {
	// Symmetric XOR case: providing only stop (no start) must also return 400.
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"stop": "2026-06-15T10:00:00Z"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("stop-only status = %d, want 400", res.StatusCode)
	}
}

func TestLiveStart_StillWorks(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"tag": "live"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("live start status = %d, want 201", res.StatusCode)
	}
}

func TestHandleListSessions_Pagination(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Seed 3 sessions via the HTTP layer so they land under the authed user's ID.
	times := [][2]string{
		{"2026-06-15T08:00:00Z", "2026-06-15T09:00:00Z"},
		{"2026-06-15T10:00:00Z", "2026-06-15T11:00:00Z"},
		{"2026-06-15T12:00:00Z", "2026-06-15T13:00:00Z"},
	}
	for _, p := range times {
		res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"start": p[0], "stop": p[1]})
		if res.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("seed status = %d (%s)", res.StatusCode, b)
		}
		_ = res.Body.Close()
	}

	// GET with ?limit=2&offset=0 — expect 2 items, X-Total-Count=3.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sessions?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d (%s), want 200", res.StatusCode, b)
	}
	if got := res.Header.Get("X-Total-Count"); got != "3" {
		t.Fatalf("X-Total-Count = %q, want 3", got)
	}
	var out []domain.WorkSession
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
}

func TestHandleListSessions_BadLimit(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	for _, bad := range []string{"0", "201", "abc", "-1"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sessions?limit="+bad, nil)
		req.Header.Set("Authorization", "Bearer x")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET limit=%s: %v", bad, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("limit=%s: status = %d, want 400", bad, res.StatusCode)
		}
	}
}

func TestEditSession_OverlapConflict(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Backfill session A: 09:00–11:00.
	resA := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T11:00:00Z",
	})
	if resA.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resA.Body)
		t.Fatalf("seed A status = %d (%s)", resA.StatusCode, b)
	}
	_ = resA.Body.Close()

	// Backfill session B: 13:00–15:00.
	var sessionB domain.WorkSession
	resB := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T13:00:00Z", "stop": "2026-06-15T15:00:00Z",
	})
	if resB.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resB.Body)
		t.Fatalf("seed B status = %d (%s)", resB.StatusCode, b)
	}
	_ = json.NewDecoder(resB.Body).Decode(&sessionB)
	_ = resB.Body.Close()

	// PATCH session B onto A's interval → should get 409.
	patchBody, _ := json.Marshal(map[string]any{
		"start": "2026-06-15T10:00:00Z", "stop": "2026-06-15T10:30:00Z",
	})
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/sessions/"+sessionB.ID, bytes.NewReader(patchBody))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("overlap edit status = %d, want 409", res.StatusCode)
	}
}

// newReassignServer builds a server with BulkAssignNode wired in addition to
// the standard worktime usecases. Returns the server, the session store, and the
// project store so the test can seed data.
func newReassignServer(t *testing.T) (*httpserver.Server, *testutil.FakeSessionStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	ids := &testutil.FakeIDGen{}
	srv := &httpserver.Server{
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               sse.NewBus(),
		Clock:             clk,
		Dev:               true,
		StartSession:      usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessions:      usecase.ListSessions{Sessions: sessions, Clock: clk},
		AddSession:        usecase.AddSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: sessions},
		EditSession:       usecase.EditSession{Sessions: sessions},
		ListSessionsPage:  usecase.ListSessionsPage{Sessions: sessions},
		BulkAssignNode: usecase.BulkAssignNode{Sessions: sessions, Nodes: ps},
	}
	return srv, sessions, ps
}

func TestHandleReassignSessions(t *testing.T) {
	srv, sessions, ps := newReassignServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Create a session via the HTTP API. The FakeIDGen is shared: first NewID()
	// call goes to EnsureUser (user "id-1"), second to the session ID ("id-2").
	// OwnerID is json:"-" so we cannot read it from the response; use "id-1".
	const ownerID = "id-1"
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T10:00:00Z", "tag": "work",
	})
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("seed session status = %d (%s)", res.StatusCode, b)
	}
	var created domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&created)
	_ = res.Body.Close()

	// Seed a project owned by the same user (ownerID is deterministic from FakeIDGen).
	ctx := context.Background()
	if _, err := ps.Create(ctx, domain.Node{ID: "p1", OwnerID: ownerID, Name: "flow"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Reassign the session.
	reassignRes := authPost(t, ts.URL+"/api/v1/sessions/reassign", map[string]any{
		"ids": []string{created.ID}, "projectId": "p1",
	})
	if reassignRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(reassignRes.Body)
		t.Fatalf("reassign status = %d (%s)", reassignRes.StatusCode, b)
	}
	var out map[string]int
	_ = json.NewDecoder(reassignRes.Body).Decode(&out)
	_ = reassignRes.Body.Close()
	if out["updated"] != 1 {
		t.Fatalf("updated = %d, want 1", out["updated"])
	}

	// Verify via the store directly.
	got, err := sessions.Get(ctx, ownerID, created.ID)
	if err != nil {
		t.Fatalf("store Get: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "p1" {
		t.Fatalf("session not reassigned: %+v", got)
	}
}

func TestHandleReassignSessions_EmptyIDs(t *testing.T) {
	srv, _, _ := newReassignServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions/reassign", map[string]any{
		"ids": []string{}, "projectId": "p1",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty ids status = %d, want 400", res.StatusCode)
	}
}

func TestHandleReassignSessions_ForeignProject(t *testing.T) {
	srv, _, _ := newReassignServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions/reassign", map[string]any{
		"ids": []string{"a"}, "projectId": "nonexistent",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign project status = %d, want 404", res.StatusCode)
	}
}

// newBulkDeleteServer builds a server with BulkDeleteSessions wired.
func newBulkDeleteServer(t *testing.T) (*httpserver.Server, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	ids := &testutil.FakeIDGen{}
	srv := &httpserver.Server{
		Verifier:           testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:             usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:                sse.NewBus(),
		Clock:              clk,
		Dev:                true,
		StartSession:       usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessions:       usecase.ListSessions{Sessions: sessions, Clock: clk},
		AddSession:         usecase.AddSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessionsRange:  usecase.ListSessionsRange{Sessions: sessions},
		EditSession:        usecase.EditSession{Sessions: sessions},
		ListSessionsPage:   usecase.ListSessionsPage{Sessions: sessions},
		BulkDeleteSessions: usecase.BulkDeleteSessions{Sessions: sessions},
	}
	return srv, sessions
}

func TestHandleBulkDeleteSessions(t *testing.T) {
	srv, sessions := newBulkDeleteServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Seed a session via the HTTP API. FakeIDGen: EnsureUser→"id-1", session→"id-2".
	const ownerID = "id-1"
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T10:00:00Z", "tag": "work",
	})
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("seed session status = %d (%s)", res.StatusCode, b)
	}
	var created domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&created)
	_ = res.Body.Close()

	// Bulk-delete the session (plus a missing id that should be skipped).
	deleteRes := authPost(t, ts.URL+"/api/v1/sessions/bulk-delete", map[string]any{
		"ids": []string{created.ID, "missing"},
	})
	if deleteRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(deleteRes.Body)
		t.Fatalf("bulk-delete status = %d (%s)", deleteRes.StatusCode, b)
	}
	var out map[string]int
	_ = json.NewDecoder(deleteRes.Body).Decode(&out)
	_ = deleteRes.Body.Close()
	if out["deleted"] != 1 {
		t.Fatalf("deleted = %d, want 1", out["deleted"])
	}

	// Verify via the store: session is gone.
	ctx := context.Background()
	if _, err := sessions.Get(ctx, ownerID, created.ID); err == nil {
		t.Fatal("session still exists after bulk-delete")
	}
}

func TestHandleBulkDeleteSessions_EmptyIDs(t *testing.T) {
	srv, _ := newBulkDeleteServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions/bulk-delete", map[string]any{
		"ids": []string{},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty ids status = %d, want 400", res.StatusCode)
	}
}
