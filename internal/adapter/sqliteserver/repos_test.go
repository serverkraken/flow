package sqliteserver

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_ServerRepos_EnsureByCanonicalKey_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "srepo1")
	repos := NewRepos(store)

	a, err := repos.EnsureByCanonicalKey(u.ID, "git:gh.com/o/r", "r")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	b, err := repos.EnsureByCanonicalKey(u.ID, "git:gh.com/o/r", "different")
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if a.ID != b.ID {
		t.Errorf("ID changed: %q vs %q", a.ID, b.ID)
	}
}

func TestUnit_ServerRepos_Upsert_InsertBumpsVersion(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "srepo2")
	repos := NewRepos(store)

	got, err := repos.Upsert(domain.Repo{
		ID: "fixed-id", UserID: u.ID, CanonicalKey: "git:x/y", DisplayName: "y",
	}, 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Version == 0 {
		t.Error("Version should be non-zero after Upsert")
	}
}

func TestUnit_ServerRepos_Upsert_StaleVersionConflict(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "srepo3")
	repos := NewRepos(store)

	first, err := repos.Upsert(domain.Repo{
		ID: "stale-id", UserID: u.ID, CanonicalKey: "git:x/stale", DisplayName: "stale",
	}, 0)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	_, err = repos.Upsert(domain.Repo{
		ID: first.ID, UserID: u.ID, CanonicalKey: first.CanonicalKey,
		DisplayName: "renamed",
	}, first.Version+99)
	if !errors.Is(err, ports.ErrRepoVersionConflict) {
		t.Errorf("want ErrRepoVersionConflict, got %v", err)
	}
}

func TestUnit_ServerRepos_PullSince_AscendingOrder(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "srepo4")
	repos := NewRepos(store)

	var versions []int64
	for i := 0; i < 3; i++ {
		r, err := repos.Upsert(domain.Repo{
			ID: "ord-" + string(rune('a'+i)), UserID: u.ID,
			CanonicalKey: "git:x/ord" + string(rune('a'+i)),
			DisplayName:  "ord",
		}, 0)
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		versions = append(versions, r.Version)
	}

	got, _, _, err := repos.PullSince(u.ID, versions[0], 10)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (the two newer rows)", len(got))
	}
}
