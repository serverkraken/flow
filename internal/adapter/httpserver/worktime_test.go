package httpserver_test

import (
	"bytes"
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

func TestLiveStart_StillWorks(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"tag": "live"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("live start status = %d, want 201", res.StatusCode)
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
