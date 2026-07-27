package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestStartSessionAndListProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sessions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"s1","start":"2026-06-14T09:00:00Z"}`))
		case "/api/v1/nodes":
			_, _ = w.Write([]byte(`[{"id":"p1","name":"Flow","slug":"flow","status":"active"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	s, err := c.StartSession(context.Background(), nil, nil, "")
	if err != nil || s.ID != "s1" {
		t.Fatalf("StartSession = %+v err=%v", s, err)
	}
	ps, err := c.ListNodes(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Name != "Flow" {
		t.Fatalf("ListNodes = %+v err=%v", ps, err)
	}
}

func TestEventsStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("event: session.started\ndata: {\"type\":\"session.started\",\"data\":{\"id\":\"s1\"}}\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	ev := <-ch
	if ev.Type != "session.started" || ev.Data["id"] != "s1" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestEditAndDeleteSession(t *testing.T) {
	var sawPatch, sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/sessions/s1":
			sawPatch = true
			_, _ = w.Write([]byte(`{"id":"s1","tags":["deep"],"start":"2026-06-14T09:00:00Z"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/sessions/s1":
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	deepTags := []string{"deep"}
	s, err := c.EditSession(context.Background(), "s1", nil, &deepTags, "", start, &stop)
	if err != nil || len(s.Tags) != 1 || s.Tags[0] != "deep" {
		t.Fatalf("EditSession = %+v err=%v", s, err)
	}
	if err := c.DeleteSession(context.Background(), "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if !sawPatch || !sawDelete {
		t.Fatalf("server not hit: patch=%v delete=%v", sawPatch, sawDelete)
	}
}

func TestClient_ListSessionsSince(t *testing.T) {
	var gotSince string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"s1","start":"2026-06-01T09:00:00Z"}]`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	since := time.Date(2026, 3, 22, 8, 0, 0, 0, time.UTC)
	out, err := c.ListSessionsSince(context.Background(), since)
	if err != nil {
		t.Fatalf("ListSessionsSince: %v", err)
	}
	if len(out) != 1 || out[0].ID != "s1" {
		t.Fatalf("decoded sessions = %+v, want one with id s1", out)
	}
	if got, want := gotSince, since.Format(time.RFC3339); got != want {
		t.Errorf("since query = %q, want %q", got, want)
	}
}

func TestClient_DeleteProject(t *testing.T) {
	var sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/nodes/p1" {
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	if err := c.DeleteNode(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	if !sawDelete {
		t.Fatal("server DELETE /api/v1/nodes/p1 was not called")
	}
}

func TestAddSessionAndListRange(t *testing.T) {
	var gotBody map[string]any
	var gotRangeQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"s9","start":"2026-06-15T09:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions":
			gotRangeQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`[{"id":"s9","start":"2026-06-15T09:00:00Z"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	s, err := c.AddSession(context.Background(), nil, start, stop, []string{"deep"}, "n")
	if err != nil || s.ID != "s9" {
		t.Fatalf("AddSession = %+v err=%v", s, err)
	}
	gotTags, ok := gotBody["tags"].([]any)
	if gotBody["start"] == nil || gotBody["stop"] == nil || !ok || len(gotTags) != 1 || gotTags[0] != "deep" {
		t.Fatalf("AddSession body missing fields: %+v", gotBody)
	}

	list, err := c.ListSessionsRange(context.Background(), start, stop)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListSessionsRange = %+v err=%v", list, err)
	}
	if !strings.Contains(gotRangeQuery, "since=") || !strings.Contains(gotRangeQuery, "until=") {
		t.Fatalf("range query missing since/until: %q", gotRangeQuery)
	}
}

func TestClient_ReassignAndPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sessions/reassign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"updated": 3})
	})
	mux.HandleFunc("GET /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total-Count", "7")
		_ = json.NewEncoder(w).Encode([]domain.WorkSession{{ID: "a"}})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	if n, err := c.ReassignSessions(context.Background(), "p1", []string{"a", "b", "c"}); err != nil || n != 3 {
		t.Fatalf("reassign n=%d err=%v", n, err)
	}
	items, total, err := c.ListSessionsPage(context.Background(), 5, 0)
	if err != nil || total != 7 || len(items) != 1 {
		t.Fatalf("page items=%d total=%d err=%v", len(items), total, err)
	}
}

func TestClient_BulkDeleteSessions(t *testing.T) {
	var sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions/bulk-delete" {
			sawDelete = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{"deleted": 2})
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	n, err := c.BulkDeleteSessions(context.Background(), []string{"s1", "s2"})
	if err != nil || n != 2 {
		t.Fatalf("BulkDeleteSessions n=%d err=%v", n, err)
	}
	if !sawDelete {
		t.Fatal("server POST /api/v1/sessions/bulk-delete was not called")
	}
}
