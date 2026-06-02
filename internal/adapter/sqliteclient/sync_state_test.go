package sqliteclient

import (
	"testing"
)

func TestUnit_SyncState_Get_ZeroForMissingResource(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	ss := NewSyncState(store)

	w, err := ss.Get("sessions")
	if err != nil {
		t.Fatalf("Get for missing resource: %v", err)
	}
	if w != 0 {
		t.Errorf("expected watermark 0 for missing resource, got %d", w)
	}
}

func TestUnit_SyncState_Set_CreatesAndUpdates(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	ss := NewSyncState(store)

	if err := ss.Set("sessions", 42); err != nil {
		t.Fatalf("Set (create): %v", err)
	}

	w, err := ss.Get("sessions")
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if w != 42 {
		t.Errorf("expected 42, got %d", w)
	}

	// Second Set on same resource must update, not insert.
	if err := ss.Set("sessions", 99); err != nil {
		t.Fatalf("Set (update): %v", err)
	}

	w2, err := ss.Get("sessions")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if w2 != 99 {
		t.Errorf("expected 99 after update, got %d", w2)
	}
}

func TestUnit_SyncState_MultipleResources_IndependentWatermarks(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	ss := NewSyncState(store)

	if err := ss.Set("sessions", 10); err != nil {
		t.Fatalf("Set sessions: %v", err)
	}
	if err := ss.Set("projects", 20); err != nil {
		t.Fatalf("Set projects: %v", err)
	}

	wSessions, err := ss.Get("sessions")
	if err != nil {
		t.Fatalf("Get sessions: %v", err)
	}
	wProjects, err := ss.Get("projects")
	if err != nil {
		t.Fatalf("Get projects: %v", err)
	}

	if wSessions != 10 {
		t.Errorf("sessions watermark: got %d, want 10", wSessions)
	}
	if wProjects != 20 {
		t.Errorf("projects watermark: got %d, want 20", wProjects)
	}
}
