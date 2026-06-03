package sqliteserver

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// serverTestUser provisions a user for FK satisfaction in server tests.
func serverTestUser(t *testing.T, store *Store, subSuffix string) domain.User {
	t.Helper()
	u, err := NewUsers(store).EnsureBySub("sub|"+subSuffix, subSuffix+"@example.com", subSuffix)
	if err != nil {
		t.Fatalf("serverTestUser EnsureBySub: %v", err)
	}
	return u
}

// serverTestProject provisions a project for FK satisfaction in server tests.
func serverTestProject(t *testing.T, store *Store, userID, slug string) domain.Project {
	t.Helper()
	p, err := NewProjects(store).EnsureBySlug(userID, slug, slug)
	if err != nil {
		t.Fatalf("serverTestProject EnsureBySlug: %v", err)
	}
	return p
}

func TestUnit_ServerProjects_EnsureBySlug_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	users := NewUsers(store)
	projects := NewProjects(store)

	u, err := users.EnsureBySub("sub|sproj1", "user@example.com", "User")
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
		t.Errorf("ID changed: %q -> %q", p1.ID, p2.ID)
	}
	if p2.Name != "My Project" {
		t.Errorf("name overwritten: got %q, want %q", p2.Name, "My Project")
	}
}

func TestUnit_ServerProjects_PullSince_ReturnsOnlyGreaterVersions(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj2")
	projects := NewProjects(store)

	// Insert 3 projects via Upsert — each gets a unique Lamport version.
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	now := time.Now().UTC()
	var versions []int64
	for _, id := range ids {
		p, err := projects.Upsert(domain.Project{
			ID:        id,
			UserID:    u.ID,
			Name:      "proj-" + id[:8],
			Slug:      "slug-" + id[:8],
			CreatedAt: now,
		}, 0)
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		versions = append(versions, p.Version)
	}

	// PullSince watermark = versions[0] should return rows 1 and 2 only.
	got, high, hasMore, err := projects.PullSince(u.ID, versions[0], 10)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 rows, got %d", len(got))
	}
	if hasMore {
		t.Errorf("hasMore should be false")
	}
	if high != versions[2] {
		t.Errorf("high watermark: got %d, want %d", high, versions[2])
	}
}

func TestUnit_ServerProjects_PullSince_HasMore(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj3")
	projects := NewProjects(store)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if _, err := projects.Upsert(domain.Project{
			ID:        uuid.NewString(),
			UserID:    u.ID,
			Name:      "p",
			Slug:      uuid.NewString(),
			CreatedAt: now,
		}, 0); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	_, _, hasMore, err := projects.PullSince(u.ID, 0, 3)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if !hasMore {
		t.Errorf("hasMore should be true when more rows exist beyond limit")
	}
}

func TestUnit_ServerProjects_Upsert_Insert_Succeeds(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj4")
	projects := NewProjects(store)

	id := uuid.NewString()
	now := time.Now().UTC()
	p, err := projects.Upsert(domain.Project{
		ID:        id,
		UserID:    u.ID,
		Name:      "Test",
		Slug:      "test",
		CreatedAt: now,
	}, 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}
	if p.Version == 0 {
		t.Errorf("version should be non-zero after insert")
	}
	if p.ID != id {
		t.Errorf("ID mismatch: got %q, want %q", p.ID, id)
	}
}

func TestUnit_ServerProjects_Upsert_Update_CorrectVersion_Succeeds(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj5")
	projects := NewProjects(store)

	now := time.Now().UTC()
	p1, err := projects.Upsert(domain.Project{
		ID: uuid.NewString(), UserID: u.ID, Name: "A", Slug: "a", CreatedAt: now,
	}, 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	p2, err := projects.Upsert(domain.Project{
		ID: p1.ID, UserID: u.ID, Name: "A-updated", Slug: "a", CreatedAt: now,
	}, p1.Version)
	if err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	if p2.Version <= p1.Version {
		t.Errorf("version must bump: v1=%d v2=%d", p1.Version, p2.Version)
	}
}

func TestUnit_ServerProjects_Upsert_Update_WrongVersion_ConflictError(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj6")
	projects := NewProjects(store)

	now := time.Now().UTC()
	p1, err := projects.Upsert(domain.Project{
		ID: uuid.NewString(), UserID: u.ID, Name: "B", Slug: "b", CreatedAt: now,
	}, 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	_, err = projects.Upsert(domain.Project{
		ID: p1.ID, UserID: u.ID, Name: "B-wrong", Slug: "b", CreatedAt: now,
	}, p1.Version+99)
	if !errors.Is(err, ports.ErrProjectVersionConflict) {
		t.Errorf("want ErrProjectVersionConflict, got %v", err)
	}
}

func TestUnit_ServerProjects_Upsert_InsertWithNonZeroExpected_ConflictError(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj7")
	projects := NewProjects(store)

	now := time.Now().UTC()
	_, err := projects.Upsert(domain.Project{
		ID: uuid.NewString(), UserID: u.ID, Name: "C", Slug: "c", CreatedAt: now,
	}, 42)
	if !errors.Is(err, ports.ErrProjectVersionConflict) {
		t.Errorf("want ErrProjectVersionConflict for non-zero expected on insert, got %v", err)
	}
}

func TestUnit_ServerProjects_Upsert_Concurrent_DistinctVersions(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj8")
	projects := NewProjects(store)

	now := time.Now().UTC()
	var mu sync.Mutex
	versions := make(map[int64]bool)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := projects.Upsert(domain.Project{
				ID:        uuid.NewString(),
				UserID:    u.ID,
				Name:      "x",
				Slug:      uuid.NewString(),
				CreatedAt: now,
			}, 0)
			if err != nil {
				t.Errorf("concurrent Upsert: %v", err)
				return
			}
			mu.Lock()
			if versions[p.Version] {
				t.Errorf("duplicate version %d", p.Version)
			}
			versions[p.Version] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
}

func TestUnit_ServerProjects_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sproj9")
	projects := NewProjects(store)

	_, err := projects.GetByID(u.ID, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("want ErrProjectNotFound, got %v", err)
	}
}
