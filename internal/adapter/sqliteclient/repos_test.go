package sqliteclient

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Repos_EnsureByCanonicalKey_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "repo1")
	repos := NewRepos(store)

	r1, err := repos.EnsureByCanonicalKey(u.ID, "git:github.com/foo/bar", "bar")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	r2, err := repos.EnsureByCanonicalKey(u.ID, "git:github.com/foo/bar", "different name")
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if r1.ID != r2.ID {
		t.Errorf("ID changed: %q vs %q", r1.ID, r2.ID)
	}
	if r2.DisplayName != "bar" {
		t.Errorf("DisplayName clobbered on second call: %q", r2.DisplayName)
	}
}

func TestUnit_Repos_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "repo2")
	repos := NewRepos(store)

	_, err := repos.GetByID(u.ID, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ports.ErrRepoNotFound) {
		t.Errorf("want ErrRepoNotFound, got %v", err)
	}
}

func TestUnit_Repos_Upsert_ReplacesDisplayNameAndVersion(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "repo3")
	repos := NewRepos(store)

	orig, err := repos.EnsureByCanonicalKey(u.ID, "git:github.com/foo/orig", "orig")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	updated := domain.Repo{
		ID:           orig.ID,
		UserID:       u.ID,
		CanonicalKey: orig.CanonicalKey,
		DisplayName:  "renamed",
		CreatedAt:    orig.CreatedAt,
		Version:      7,
	}
	if err := repos.Upsert(updated); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repos.GetByID(u.ID, orig.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.DisplayName != "renamed" || got.Version != 7 {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestUnit_Repos_PullSince_ReturnsOnlyGreaterVersions(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "repo4")
	repos := NewRepos(store)

	now := time.Now().UTC()
	for i, v := range []int64{1, 3, 5, 8} {
		r := domain.Repo{
			ID:           "11111111-1111-1111-1111-00000000000" + string(rune('a'+i)),
			UserID:       u.ID,
			CanonicalKey: "git:example.com/r" + string(rune('a'+i)),
			DisplayName:  "r" + string(rune('a'+i)),
			CreatedAt:    now,
			Version:      v,
		}
		if err := repos.Upsert(r); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got, high, hasMore, err := repos.PullSince(u.ID, 3, 10)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (versions 5 and 8)", len(got))
	}
	if high != 8 {
		t.Errorf("high = %d, want 8", high)
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}

	// Versions must be ascending.
	for i := 1; i < len(got); i++ {
		if got[i].Version <= got[i-1].Version {
			t.Errorf("order broken: %d <= %d", got[i].Version, got[i-1].Version)
		}
	}
}

func TestUnit_Repos_PullSince_HasMoreFlag(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "repo5")
	repos := NewRepos(store)

	now := time.Now().UTC()
	for i := 1; i <= 4; i++ {
		r := domain.Repo{
			ID:           "22222222-2222-2222-2222-00000000000" + string(rune('0'+i)),
			UserID:       u.ID,
			CanonicalKey: "git:example.com/big" + string(rune('0'+i)),
			DisplayName:  "big",
			CreatedAt:    now,
			Version:      int64(i),
		}
		if err := repos.Upsert(r); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got, _, hasMore, err := repos.PullSince(u.ID, 0, 2)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
	if !hasMore {
		t.Error("hasMore = false, want true")
	}
}
