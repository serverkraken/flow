package httpapi_test

import (
	"errors"
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
