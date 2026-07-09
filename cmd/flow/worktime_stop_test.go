package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/statuscache"
)

// stopTestServer serves ListNodes / GetNode / StopSession and records the last
// projectId a stop booked to. Nodes: n-repo (slug repo-slug), n1 (slug flow).
func stopTestServer(t *testing.T, stopStatus int) (*httptest.Server, *string) {
	t.Helper()
	var lastBookedTo string
	nodesJSON := `[{"id":"n-repo","ownerId":"u1","name":"Flow Repo","kind":"repo","slug":"repo-slug"},{"id":"n1","ownerId":"u1","name":"Flow","kind":"repo","slug":"flow"}]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if pid, ok := body["projectId"].(string); ok {
				lastBookedTo = pid
			}
			if stopStatus != http.StatusOK {
				http.Error(w, "boom", stopStatus)
				return
			}
			_, _ = w.Write([]byte(`{"id":"s1","ownerId":"u1","start":"2026-07-08T09:00:00Z","stop":"2026-07-08T11:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(nodesJSON))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/nodes/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
			_, _ = w.Write([]byte(`{"id":"` + id + `","ownerId":"u1","name":"Flow","kind":"repo","slug":"flow"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &lastBookedTo
}

func fetchRunning(id string, nodeID *string) func(context.Context) (apiclient.WorktimeStatus, error) {
	return func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{Running: true, ActiveSessionID: id, ActiveNodeID: nodeID}, nil
	}
}

func TestRunStop_NoRunningSession(t *testing.T) {
	var out bytes.Buffer
	st := apiclient.WorktimeStatus{Running: false}
	err := runStop(context.Background(), nil, func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", false, nil, &out)
	if err != nil {
		t.Fatalf("no-session must not error: %v", err)
	}
	if !strings.Contains(out.String(), "kein") { // "keine laufende Session"
		t.Errorf("expected no-session note, got %q", out.String())
	}
}

func TestRunStop_NonTTYWithoutNodeErrors(t *testing.T) {
	st := apiclient.WorktimeStatus{Running: true, ActiveSessionID: "s1"} // no ActiveNodeID
	err := runStop(context.Background(), nil, func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", false /*interactive*/, nil, io.Discard)
	if err == nil {
		t.Fatal("non-tty unbooked stop without --node must error, not hang")
	}
}

// Finding C12: a stop that fails AFTER a node was chosen must propagate the
// error AND leave the cache untouched (so the segment still shows the timer).
func TestRunStop_StopFailureKeepsCacheAndErrors(t *testing.T) {
	ts, _ := stopTestServer(t, http.StatusInternalServerError)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	_ = statuscache.Write(statusCachePath(), statuscache.Entry{FetchedAt: time.Now()})
	c := apiclient.New(ts.URL, "tok")
	st := apiclient.WorktimeStatus{Running: true, ActiveSessionID: "s1"}
	err := runStop(context.Background(), c,
		func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", true, // interactive
		func(context.Context, *apiclient.Client) (string, error) { return "n1", nil }, // picker returns a node
		io.Discard)
	if err == nil {
		t.Fatal("stop failure must propagate")
	}
	if _, ok := statuscache.Read(statusCachePath()); !ok {
		t.Error("cache must NOT be invalidated when stop fails")
	}
}

// A running session already booked at start stops straight away (no picker) and
// books to its ActiveNodeID.
func TestRunStop_ActiveNodeIDDirectStop(t *testing.T) {
	ts, booked := stopTestServer(t, http.StatusOK)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := apiclient.New(ts.URL, "tok")
	node := "n1"
	var out bytes.Buffer
	err := runStop(context.Background(), c, fetchRunning("s1", &node), "", false, nil, &out)
	if err != nil {
		t.Fatalf("direct stop must succeed: %v", err)
	}
	if *booked != "n1" {
		t.Errorf("must book to ActiveNodeID n1, booked %q", *booked)
	}
	if !strings.Contains(out.String(), "gestoppt") {
		t.Errorf("expected confirmation, got %q", out.String())
	}
}

// --node resolves and stops non-interactively.
func TestRunStop_NodeFlagNonTTY(t *testing.T) {
	ts, booked := stopTestServer(t, http.StatusOK)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := apiclient.New(ts.URL, "tok")
	err := runStop(context.Background(), c, fetchRunning("s1", nil), "repo-slug", false, nil, io.Discard)
	if err != nil {
		t.Fatalf("--node non-tty must succeed: %v", err)
	}
	if *booked != "n-repo" {
		t.Errorf("--node repo-slug should book to n-repo, booked %q", *booked)
	}
}

// Finding #4: --node wins over an already-booked ActiveNodeID (rebooking).
func TestRunStop_NodeFlagOverridesActiveNodeID(t *testing.T) {
	ts, booked := stopTestServer(t, http.StatusOK)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := apiclient.New(ts.URL, "tok")
	active := "n1"
	err := runStop(context.Background(), c, fetchRunning("s1", &active), "repo-slug", false, nil, io.Discard)
	if err != nil {
		t.Fatalf("rebooking must succeed: %v", err)
	}
	if *booked != "n-repo" {
		t.Errorf("--node must override ActiveNodeID: booked %q, want n-repo", *booked)
	}
}

func TestRunStop_BadNodeRefErrors(t *testing.T) {
	ts, booked := stopTestServer(t, http.StatusOK)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := apiclient.New(ts.URL, "tok")
	err := runStop(context.Background(), c, fetchRunning("s1", nil), "nonexistent", false, nil, io.Discard)
	if err == nil {
		t.Fatal("unknown --node ref must error")
	}
	if *booked != "" {
		t.Errorf("no stop should have happened, booked %q", *booked)
	}
}

// Picker Esc (pick returns "" ,nil) must NOT stop the session.
func TestRunStop_PickerCancelDoesNotStop(t *testing.T) {
	ts, booked := stopTestServer(t, http.StatusOK)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := apiclient.New(ts.URL, "tok")
	err := runStop(context.Background(), c, fetchRunning("s1", nil), "", true,
		func(context.Context, *apiclient.Client) (string, error) { return "", nil }, // Esc
		io.Discard)
	if err != nil {
		t.Fatalf("cancel must not error: %v", err)
	}
	if *booked != "" {
		t.Errorf("cancel must not stop; booked %q", *booked)
	}
}
