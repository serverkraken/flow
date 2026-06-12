package httpapi_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestActiveSessions_ListByUser(t *testing.T) {
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	// Initially there should be no active sessions (clean test DB per test run)
	all, err := active.ListByUser("")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	// Should return empty or existing; no assertion on count, just no error
	_ = all
}

func TestActiveSessions_Get_NotFound(t *testing.T) {
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	_, err := active.Get("", "nonexistent-project-id")
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("expected ErrActiveSessionNotFound, got: %v", err)
	}
}

func TestActiveSessions_Upsert_Error(t *testing.T) {
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	err := active.Upsert(domain.ActiveSession{})
	if err == nil {
		t.Fatal("expected error for Upsert on read-only active sessions, got nil")
	}
}

func TestActiveSessions_Delete_Error(t *testing.T) {
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	err := active.Delete("", "any-project-id")
	if err == nil {
		t.Fatal("expected error for Delete on read-only active sessions, got nil")
	}
}

// TestActiveSessions_StaleSnapshot_NotUsedAsCache is a regression test for the
// cross-device sync bug: a session started on device A and stopped on device B
// (e.g. via WebUI) was shown as still running by device A's CLI because the
// snapshot was pre-populated into the cache on each new process start.
// After the fix, a snapshot with a running session must NOT cause a cache hit —
// the server is always queried first; snapshot data is offline fallback only.
func TestActiveSessions_StaleSnapshot_NotUsedAsCache(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	// Write a snapshot with a fake active session to simulate a session that
	// was stopped on another device while this device's snapshot is stale.
	snapDir := filepath.Join(stateDir, "flow")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	staleSnap := `{"active":[{"project_id":"ghost-project","started_at":"2026-01-01T09:00:00Z","version":1}]}`
	if err := os.WriteFile(filepath.Join(snapDir, "snapshot.json"), []byte(staleSnap), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	// Server has no active sessions — the session was stopped via WebUI.
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	result, err := active.ListByUser("")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("stale snapshot returned as cache hit: got %d active sessions, want 0 (server truth)", len(result))
	}
}

func TestActiveSessions_Offline_FallsBackToCache(t *testing.T) {
	api := newTestAPI(t)
	active := httpapi.NewActiveSessions(api.Client)

	// Populate cache with an initial read
	first, err := active.ListByUser("")
	if err != nil {
		t.Fatalf("initial ListByUser: %v", err)
	}

	// Kill the server
	api.Close()

	// Should return cached data (empty or not)
	second, err := active.ListByUser("")
	if err != nil {
		t.Fatalf("offline ListByUser returned error: %v", err)
	}
	if len(second) != len(first) {
		t.Errorf("offline: got %d active sessions, want %d (from cache)", len(second), len(first))
	}
}
