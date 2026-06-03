package sqliteclient

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Projects_EnsureBySlug_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|proj1", "user@example.com", "User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	p1, err := projects.EnsureBySlug(u.ID, "My Project", "my-project")
	if err != nil {
		t.Fatalf("first EnsureBySlug: %v", err)
	}

	p2, err := projects.EnsureBySlug(u.ID, "Different Name", "my-project")
	if err != nil {
		t.Fatalf("second EnsureBySlug: %v", err)
	}

	if p1.ID != p2.ID {
		t.Errorf("ID changed: %q → %q", p1.ID, p2.ID)
	}
	if p2.Name != "My Project" {
		t.Errorf("name overwritten: got %q, want %q", p2.Name, "My Project")
	}
}

func TestUnit_Projects_ListActive_MRUSorted(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|proj2", "user@example.com", "User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	pA, err := projects.EnsureBySlug(u.ID, "Alpha", "alpha")
	if err != nil {
		t.Fatalf("EnsureBySlug alpha: %v", err)
	}
	pB, err := projects.EnsureBySlug(u.ID, "Beta", "beta")
	if err != nil {
		t.Fatalf("EnsureBySlug beta: %v", err)
	}

	// Touch alpha first, then beta — beta should appear first in MRU order.
	if err := projects.TouchLastUsed(u.ID, pA.ID); err != nil {
		t.Fatalf("TouchLastUsed alpha: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := projects.TouchLastUsed(u.ID, pB.ID); err != nil {
		t.Fatalf("TouchLastUsed beta: %v", err)
	}

	list, err := projects.ListActive(u.ID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 projects, got %d", len(list))
	}
	if list[0].ID != pB.ID {
		t.Errorf("MRU first: got %q, want %q (beta)", list[0].Slug, "beta")
	}
	if list[1].ID != pA.ID {
		t.Errorf("MRU second: got %q, want %q (alpha)", list[1].Slug, "alpha")
	}
}

func TestUnit_Projects_Archive_HidesFromListActive(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|proj3", "user@example.com", "User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	p, err := projects.EnsureBySlug(u.ID, "ToArchive", "to-archive")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	if err := projects.Archive(u.ID, p.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	active, err := projects.ListActive(u.ID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	for _, ap := range active {
		if ap.ID == p.ID {
			t.Errorf("archived project %q should not appear in ListActive", p.Slug)
		}
	}

	all, err := projects.ListAll(u.ID)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	found := false
	for _, ap := range all {
		if ap.ID == p.ID {
			found = true
			if ap.ArchivedAt == nil {
				t.Errorf("archived_at should be set after Archive()")
			}
		}
	}
	if !found {
		t.Errorf("archived project %q should appear in ListAll", p.Slug)
	}
}

func TestUnit_Projects_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|proj4", "user@example.com", "User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	_, err = projects.GetByID(u.ID, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("want ErrProjectNotFound, got %v", err)
	}
}

func TestUnit_Projects_Upsert_InsertAndUpdate(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|proj-up", "user@example.com", "User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	// Insert path — Upsert against an id that does not exist yet.
	archived := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	now := time.Now().UTC().Truncate(time.Second)
	inserted := domain.Project{
		ID:         "11111111-1111-1111-1111-111111111111",
		UserID:     u.ID,
		Name:       "Orig",
		Slug:       "orig",
		CreatedAt:  now,
		LastUsedAt: now,
		ArchivedAt: &archived,
		Version:    7,
	}
	if err := projects.Upsert(inserted); err != nil {
		t.Fatalf("Upsert insert: %v", err)
	}
	got, err := projects.GetByID(u.ID, inserted.ID)
	if err != nil {
		t.Fatalf("GetByID after insert: %v", err)
	}
	if got.Name != "Orig" || got.Slug != "orig" || got.Version != 7 {
		t.Errorf("inserted mismatch: %+v", got)
	}
	if got.ArchivedAt == nil {
		t.Error("ArchivedAt nil after insert with non-nil archived_at")
	}

	// Update path — same id, new values.
	updated := domain.Project{
		ID:         inserted.ID,
		UserID:     u.ID,
		Name:       "Renamed",
		Slug:       "renamed",
		CreatedAt:  got.CreatedAt,
		LastUsedAt: now.Add(time.Minute),
		ArchivedAt: nil,
		Version:    9,
	}
	if err := projects.Upsert(updated); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	got2, err := projects.GetByID(u.ID, inserted.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got2.Name != "Renamed" || got2.Slug != "renamed" || got2.Version != 9 {
		t.Errorf("updated mismatch: %+v", got2)
	}
	if got2.ArchivedAt != nil {
		t.Error("ArchivedAt not cleared by update with nil archived_at")
	}
}
