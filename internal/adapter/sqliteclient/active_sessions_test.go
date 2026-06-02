package sqliteclient

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_ActiveSessions_Upsert_InsertAndReplace(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)
	active := NewActiveSessions(store)

	u, err := users.EnsureBySub("sub|as1", "as1@example.com", "AS1")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	p, err := projects.EnsureBySlug(u.ID, "as-proj", "as-proj")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	as1 := domain.ActiveSession{
		UserID:          u.ID,
		ProjectID:       p.ID,
		StartedAt:       now,
		StartedOnDevice: "laptop",
		Version:         1,
	}
	if err := active.Upsert(as1); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	got, err := active.Get(u.ID, p.ID)
	if err != nil {
		t.Fatalf("Get after insert: %v", err)
	}
	if got.StartedOnDevice != "laptop" {
		t.Errorf("StartedOnDevice: got %q, want %q", got.StartedOnDevice, "laptop")
	}
	if !got.StartedAt.Equal(now) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, now)
	}

	// Replace with updated values.
	later := now.Add(5 * time.Minute)
	as2 := domain.ActiveSession{
		UserID:          u.ID,
		ProjectID:       p.ID,
		StartedAt:       later,
		StartedOnDevice: "workstation",
		Version:         2,
	}
	if err := active.Upsert(as2); err != nil {
		t.Fatalf("Upsert (replace): %v", err)
	}

	got2, err := active.Get(u.ID, p.ID)
	if err != nil {
		t.Fatalf("Get after replace: %v", err)
	}
	if got2.StartedOnDevice != "workstation" {
		t.Errorf("StartedOnDevice after replace: got %q, want %q", got2.StartedOnDevice, "workstation")
	}
	if got2.Version != 2 {
		t.Errorf("Version after replace: got %d, want 2", got2.Version)
	}
}

func TestUnit_ActiveSessions_Delete_ThenGetReturnsNotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)
	active := NewActiveSessions(store)

	u, err := users.EnsureBySub("sub|as2", "as2@example.com", "AS2")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	p, err := projects.EnsureBySlug(u.ID, "as-del", "as-del")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := active.Upsert(domain.ActiveSession{
		UserID:          u.ID,
		ProjectID:       p.ID,
		StartedAt:       now,
		StartedOnDevice: "laptop",
		Version:         1,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := active.Delete(u.ID, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = active.Get(u.ID, p.ID)
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("want ErrActiveSessionNotFound after Delete, got %v", err)
	}
}

func TestUnit_ActiveSessions_ListByUser_MultipleParallel(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)
	active := NewActiveSessions(store)

	u, err := users.EnsureBySub("sub|as3", "as3@example.com", "AS3")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	// Create 3 projects, each with an active session.
	for i, slug := range []string{"as-multi1", "as-multi2", "as-multi3"} {
		p, err := projects.EnsureBySlug(u.ID, slug, slug)
		if err != nil {
			t.Fatalf("EnsureBySlug %q: %v", slug, err)
		}
		if err := active.Upsert(domain.ActiveSession{
			UserID:          u.ID,
			ProjectID:       p.ID,
			StartedAt:       now.Add(time.Duration(i) * time.Minute),
			StartedOnDevice: "laptop",
			Version:         1,
		}); err != nil {
			t.Fatalf("Upsert %q: %v", slug, err)
		}
	}

	list, err := active.ListByUser(u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 parallel active sessions, got %d", len(list))
	}
}
